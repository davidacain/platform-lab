# k8s-resource-inspector

#project #go #kubernetes #in-progress

**Status:** Not started
**Priority:** Second — builds on pod-security-inspector client-go foundation
**Binary:** `kri`
**Repo:** github.com/davidacain/platform-lab/tools/k8s-resource-inspector

---

## Description

CLI tool that analyzes Kubernetes workload resource utilization by combining live pod metrics from Prometheus with current resource configuration read from ArgoCD Application CRs and Git values files. Classifies workload behavior, validates HPA configuration, and surfaces rightsizing recommendations — without making any changes to the cluster or Git.

Runs on the ArgoCD control cluster. Reaches each managed cluster's Prometheus via internal load balancer. Reads ArgoCD Application CRs to discover workloads and join them to the correct Prometheus instance.

## Why This Project

- Solves a real operational problem — resource rightsizing is tedious and error-prone by hand
- ArgoCD + Prometheus integration demonstrates realistic platform engineering patterns
- Teaches Prometheus client, ArgoCD CRD client, client-go, goroutines, and PromQL from Go
- v2 adds PR generation — v1 validates the analysis layer first

## Scope — v1

**In scope:**
- Running pods only (completed, failed, and GC'd pods are v2)
- Read-only — no cluster changes, no Git writes, no PRs
- Terminal table output and JSON output
- ArgoCD Application CR as the source for repo/values file mapping and cluster assignment
- Prometheus (per managed cluster, via internal LB) as the source for actual usage metrics

**Out of scope for v1:**
- PR generation
- Git write-back
- Completed or garbage-collected pod analysis
- VPA integration

---

## Architecture

```
                    ┌─────────────────┐
                    │   ArgoCD CRs    │  source repo, values paths, target cluster name
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │   Git (values)  │  current requests/limits per workload
                    └────────┬────────┘
                             │
┌──────────────────┐  ┌──────▼──────────┐
│  Prometheus      ├──►   Analyzer      │  classify behavior, validate HPA
│  (per cluster,   │  └──────┬──────────┘
│  internal LB)    │         │
└──────────────────┘  ┌──────▼──────────┐
                       │  Output/Report  │  table, JSON
                       └─────────────────┘
```

The tool runs on the ArgoCD control cluster. It does not need kubeconfig access to managed clusters — all workload discovery flows through ArgoCD Application CRs and Prometheus. This makes it resilient to a future migration from ArgoCD's push model to Argo CD Agents (pull model), where stored cluster kubeconfigs on the control plane go away.

---

## Configuration

`~/.kri/config.yaml` — required. Maps ArgoCD cluster names to Prometheus endpoints.

```yaml
clusters:
  - argocd_cluster: prod-us-east1
    prometheus: http://10.10.1.5:9090
  - argocd_cluster: prod-eu-west1
    prometheus: http://10.20.1.5:9090
  - argocd_cluster: staging
    prometheus: http://10.30.1.5:9090
```

`argocd_cluster` matches `spec.destination.name` on ArgoCD Application CRs. The tool joins on this field to route each workload's queries to the correct Prometheus instance.

Override config path with `--config`.

---

## Project Structure

```
tools/k8s-resource-inspector/
├── main.go
├── cmd/
│   ├── root.go          # cobra root + global flags
│   ├── inspect.go       # main inspect command
│   └── version.go
└── pkg/
    ├── config/
    │   └── config.go    # load ~/.kri/config.yaml, resolve prometheus per cluster
    ├── argo/
    │   └── apps.go      # read ArgoCD Application CRs, extract repo + values paths + destination cluster
    ├── git/
    │   └── values.go    # clone/read repo, parse Helm values for resource config
    ├── metrics/
    │   └── prometheus.go # PromQL queries for CPU/memory p50/p95/p99, limit proximity
    ├── pods/
    │   └── pods.go      # PodLister interface + client-go implementation
    ├── analysis/
    │   └── classify.go  # behavior classification, HPA validation, recommendation logic
    └── output/
        └── table.go     # terminal table + JSON output
```

---

## Key Libraries

- `k8s.io/client-go` — Kubernetes API client (shared with pod-security-inspector)
- `github.com/argoproj/argo-cd/v2` — ArgoCD Application CR types
- `github.com/prometheus/client_golang` — Prometheus HTTP client + PromQL
- `gopkg.in/src-d/go-git.v4` — Git repo access for values file reading (see note below)
- `github.com/spf13/cobra` — CLI framework
- `k8s.io/apimachinery` — resource quantity math (millicores, Mi/Gi)
- `github.com/Masterminds/semver` — version comparison (shared pkg/version)

> **Note on go-git:** `go-git` is slower than native git and has incomplete SSH agent support. For Phase 6, prototype both `go-git` and shelling out to `git` before committing. If repos are hosted on GitHub/GitLab, the hosting API may be preferable.

---

## Data Model

```go
// BehaviorClass classifies workload resource usage pattern.
type BehaviorClass string

const (
    BehaviorStatic  BehaviorClass = "STATIC"  // low variance, consistent usage
    BehaviorSpiky   BehaviorClass = "SPIKY"   // high variance between p50 and p99
    BehaviorGrowth  BehaviorClass = "GROWTH"  // consistent upward trend over window
    BehaviorRunaway BehaviorClass = "RUNAWAY" // p99 at or above limit
    BehaviorMixed   BehaviorClass = "MIXED"   // pods within workload disagree
    BehaviorUnknown BehaviorClass = "UNKNOWN" // insufficient data
)

// PodMetrics holds all data for a single running pod.
type PodMetrics struct {
    Namespace     string
    PodName       string
    ContainerName string
    AppName       string // ArgoCD Application name
    ClusterName   string // ArgoCD destination cluster name

    // From cluster
    CPURequest resource.Quantity
    CPULimit   resource.Quantity
    MemRequest resource.Quantity
    MemLimit   resource.Quantity

    // From Prometheus (over observation window)
    CPUP50   float64
    CPUP95   float64
    CPUP99   float64
    MemP50   float64
    MemP95   float64
    MemP99        float64
    MemTrend      float64  // bytes/hour slope over window
    MemLimitRatio float64  // MemP99 / MemLimit — primary signal for increase recommendation

    // Derived
    Behavior   BehaviorClass
    Confidence float64 // 0.0–1.0
}

// WorkloadSummary aggregates pod-level findings per workload.
type WorkloadSummary struct {
    AppName        string
    WorkloadName   string
    Namespace      string
    ClusterName    string
    Pods           []PodMetrics
    Behavior       BehaviorClass
    BehaviorBreakdown map[BehaviorClass]int // populated when Behavior == MIXED
    Confidence     float64
    HPAStatus      HPAValidation
    Recommendation string // human-readable; empty if confidence below threshold
    ValuesFilePath string // from ArgoCD Application CR (populated in Phase 6)
}
```

---

## Behavior Classification

### STATIC
- CPU p99 / p50 ratio < 1.5
- Memory p99 / p50 ratio < 1.3
- Memory trend slope near zero (< 1% of p50 per hour)
- `MemLimitRatio` (p99 / limit) below warning threshold

### SPIKY
- CPU p99 / p50 ratio ≥ 2.0 OR memory p99 / p50 ratio ≥ 1.8

### GROWTH
- Memory trend slope consistently positive over window
- p99 at end of window significantly higher than p99 at start
- Note: may be a memory leak or legitimate onboarding growth — tool surfaces the pattern, not the cause

### RUNAWAY
- `MemLimitRatio` ≥ 0.9 (p99 within 10% of or exceeding memory limit) — triggers increase recommendation regardless of other behavior
- CPU p99 consistently at or above CPU limit (throttling)

### MIXED
- Pods within the same workload classify differently
- `BehaviorBreakdown` records the distribution (e.g., `{STATIC: 2, GROWTH: 1}`)
- Recommendation is held — anomalous pod needs investigation first

### UNKNOWN
- Insufficient data points (pod age < observation window)
- Prometheus series missing or incomplete
- Minimum observation window enforced before GROWTH classification is emitted

---

## HPA Validation

| Check | Condition | Severity |
|---|---|---|
| CPU request missing | HPA targets CPU but no CPU request set | ERROR |
| Memory request missing | HPA targets memory but no memory request set | ERROR |
| Target utilization too high | HPA target % above p95 actual utilization | WARN |
| Target utilization too low | HPA target % well below p50 | WARN |
| Min replicas too low | minReplicas = 1 on SPIKY workload | WARN |
| Max replicas too low | maxReplicas hit in Prometheus history | WARN |
| VPA conflict | VPA and HPA both targeting same workload | ERROR |
| Scaling metric mismatch | CPU HPA on memory-bound workload | WARN |

---

## Output Format

### Terminal table

```
APP              WORKLOAD       POD                     BEHAVIOR  CPU_REQ  CPU_P95  MEM_REQ  MEM_P95  MEM/LIM  HPA
payments-api     payments-api   payments-api-7f8d9-abc  STATIC    500m     110m     256Mi    175Mi    68%      OK
payments-api     payments-api   payments-api-7f8d9-def  STATIC    500m     115m     256Mi    178Mi    69%      OK
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
                 SUMMARY        3 pods                  STATIC    500m     112m     256Mi    177Mi    69%           Reduce CPU to 200m, MEM to 210Mi

data-ingest      data-ingest    data-ingest-6c4f2-xyz   RUNAWAY   500m     440m     512Mi    490Mi    95%      WARN
data-ingest      data-ingest    data-ingest-6c4f2-uvw   STATIC    500m     210m     512Mi    245Mi    47%      WARN
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
                 SUMMARY        2 pods                  MIXED     —        —        512Mi    —        —        WARN  Pod xyz near limit — hold recommendation
```

### JSON (`--output json`)

Full structured output including all pod metrics, behavior breakdown, HPA findings, recommendation, and confidence score.

---

## CLI Flags

```
kri inspect [flags]

Flags:
  --window duration     Observation window for Prometheus queries (default: 7d)
  --namespace string    Scope to a single namespace (default: all)
  --app string          Scope to a single ArgoCD application
  --confidence float    Minimum confidence threshold for recommendations (default: 0.8)
  --findings-only       Only show workloads with recommendations or warnings
  --output string       Output format: table, json (default: table)
  --config string       Config file path (default: ~/.kri/config.yaml)
  --kubeconfig string   Path to kubeconfig for ArgoCD control cluster (default: ~/.kube/config)
  --context string      Kubeconfig context (default: current context)
```

---

## Build Phases

### Phase 1 — ArgoCD + pod inventory
Read ArgoCD Application CRs from the control cluster. For each app, read the destination cluster name and list running pods via Prometheus (`kube_pod_info`). Print app name, cluster, namespace, pod name, resource requests/limits.

**Done when:** `kri inspect` lists all pods grouped by ArgoCD app with current resource config.

### Phase 2 — Prometheus integration
Add Prometheus client. Resolve the correct Prometheus endpoint per workload from config. Query p50/p95/p99 CPU and memory per pod over the observation window. Add memory trend slope.

**Done when:** Table shows live usage metrics alongside requests/limits.

### Phase 3 — Behavior classification
Implement STATIC, SPIKY, GROWTH, RUNAWAY, MIXED, UNKNOWN. RUNAWAY is triggered by `MemLimitRatio` ≥ 0.9 or sustained CPU throttling — no OOMKill detection needed. Populate `BehaviorBreakdown` for MIXED workloads.

**Done when:** Each pod and workload summary shows correct behavior class.

### Phase 4 — HPA validation
Read HPA resources. Run validation checks. Surface findings in output.

**Done when:** HPA column shows OK/WARN/ERROR with explanation.

### Phase 5 — Recommendation engine
Generate recommendations for STATIC workloads above confidence threshold. Hold for MIXED, GROWTH, UNKNOWN. Include rationale in JSON output.

**Done when:** Recommendation column populated with defensible suggestions.

### Phase 6 — Git values read-back
Clone source repo from ArgoCD Application CR. Parse Helm values file to surface configured value vs recommended value with file path. Evaluate `go-git` vs shelling out to `git` before implementing.

**Done when:** Output shows current configured value, recommended value, and values file path.

### Phase 7 — Output polish
Finalize `--output json`. Ensure `--findings-only` reduces noise cleanly. Add confidence score to JSON. Handle empty `ValuesFilePath` gracefully in Phases 1–5.

**Done when:** Both output formats work cleanly end-to-end.

---

## Gotchas

- Kubernetes resource quantities are not plain integers — CPU in millicores (`500m` = 0.5 CPU), memory in binary suffixes (`128Mi`, `1Gi`). Use `resource.Quantity` from apimachinery throughout.
- Prometheus pod metrics use the `container` label — query `container!=""` to exclude pause containers.
- ArgoCD Application CRs are cluster-scoped, not namespace-scoped.
- Memory trend slope needs enough data points to be meaningful — enforce a minimum observation window before emitting GROWTH.
- HPA utilization % is calculated against requests, not limits — use requests as the baseline for all HPA validation.
- `go-git` requires authentication for private repos — handle SSH key or token injection; evaluate complexity before Phase 6.
- The `PodLister` interface in `pkg/pods/pods.go` should be defined early. In v1 it is backed by Prometheus (`kube_pod_info`). If a future Argo CD Agents migration removes direct cluster access, the analysis layer does not need to change.
