# kri-operator — Product Specification

| | |
|---|---|
| **Repository** | github.com/davidacain/platform-lab |
| **Tool** | `tools/kri-operator` |
| **Status** | Pre-implementation spec |
| **Foundation** | Evolves `tools/k8s-resource-inspector` (kri) CLI to operator model |

---

## Overview

kri-operator is a Kubernetes operator that automates two platform engineering workflows currently performed manually via the kri CLI:

1. **Resource rightsizing** — periodic inspection of workload resource utilization, generation of recommendations, and automated GitOps PR creation to apply them
2. **Rollback diagnosis** — event-driven detection of ArgoCD application degradation, automated log retrieval from Coralogix, and structured Slack notification with root cause context

The operator shares config, package infrastructure, and core logic with the existing kri CLI. The CLI remains as a developer and debug interface into the same underlying logic.

---

## Architecture

### Controller model

Built on `controller-runtime`. Two reconciliation loops sharing a single manager:

| Controller | Trigger | Cadence |
|---|---|---|
| `RightsizingController` | Schedule (cron-style) + manual annotation | Configurable interval, default 24h |
| `DiagnosisController` | ArgoCD `Application` watch → `Degraded` transition | Event-driven, immediate |

Both controllers watch `argoproj.io/v1alpha1/Application` CRDs. The manager holds shared clients: Prometheus, Coralogix, GitHub, Slack.

### Shared packages (carried from kri CLI)

| Package | Role |
|---|---|
| `pkg/argo` | ArgoCD Application CRD reads |
| `pkg/metrics` | Prometheus query client |
| `pkg/analysis` | Behavioral classification and recommendations |
| `pkg/plan` | Plan serialization |
| `pkg/gitops` | Git clone, branch, commit, push |
| `pkg/github` | PR creation and idempotency |
| `pkg/output` | Table and JSON formatting (CLI only) |

### New packages

| Package | Role |
|---|---|
| `pkg/coralogix` | Coralogix logs API client |
| `pkg/slack` | Slack Block Kit notification client |
| `pkg/diagnosis` | Log retrieval, event correlation, incident assembly |
| `pkg/operator` | Controller implementations, state annotations |

---

## Configuration

Single config file at `~/.kri/config.yaml` (CLI) or mounted as a `ConfigMap` (operator). Secrets injected via environment variables with `${VAR}` interpolation.

```yaml
# Cluster → Prometheus endpoint mapping.
# argocd_cluster matches spec.destination.name in ArgoCD Application CRs.
clusters:
  - argocd_cluster: in-cluster
    prometheus: http://prometheus.monitoring.svc:9090

# ArgoCD namespace where Application CRs live.
argocd_namespace: argocd

# Floor values for rightsizing recommendations.
minimums:
  cpu_millicores: 10
  memory_mi: 16

# Git committer identity for kri-authored commits.
git:
  author_name: kri
  author_email: kri@noreply.local

# GitHub settings.
github:
  base_branch: main
  api_url: https://api.github.com   # override for GitHub Enterprise Server

# Coralogix log query configuration.
coralogix:
  api_key: ${CORALOGIX_API_KEY}
  endpoint: https://ng-api-http.coralogix.com   # region-specific or custom domain

  # Log attribute used to join ArgoCD app name to a Coralogix log stream.
  # Attributes follow the resource.attributes.k8s_* naming convention used by
  # the OTel collector k8sattributesprocessor in this environment.
  # Set to "" to query by pod name instead (see Diagnosis section).
  app_name_field: resource.attributes.k8s_deployment_name

  severity_filter:                              # which severities to retrieve for diagnosis
    - ERROR
    - CRITICAL
  log_window_seconds: 120                       # seconds before probe failure to include in diagnosis

# Slack notification settings.
notifications:
  slack:
    webhook_url: ${SLACK_WEBHOOK_URL}
    channel: "#platform-alerts"

# Operator runtime settings (ignored by CLI).
operator:
  requeue_interval: 24h          # rightsizing inspection cadence
  diagnosis_cooldown: 30m        # minimum time between diagnoses for the same app
  ignore_apps: []                # ArgoCD app names to skip entirely
  ignore_namespaces: []          # namespaces to skip entirely
```

### Attribute naming convention

Coralogix log attributes in this environment follow the OTel collector
`k8sattributesprocessor` output format, prefixed with `resource.attributes.`
and using underscores in place of dots:

| OTel semantic convention | Coralogix attribute name |
|---|---|
| `k8s.cluster.name` | `resource.attributes.k8s_cluster_name` |
| `k8s.container.name` | `resource.attributes.k8s_container_name` |
| `k8s.deployment.name` | `resource.attributes.k8s_deployment_name` |
| `k8s.namespace.name` | `resource.attributes.k8s_namespace_name` |
| `k8s.pod.name` | `resource.attributes.k8s_pod_name` |

Ensure the collector config extracts at minimum:

```yaml
processors:
  k8sattributes:
    extract:
      metadata:
        - k8s.deployment.name
        - k8s.namespace.name
        - k8s.pod.name
        - k8s.container.name
        - k8s.cluster.name
```

**ArgoCD app name vs Deployment name:** `app_name_field` queries by Deployment name.
Verify that the Deployment name in each managed cluster matches `spec.destination.name`
on the ArgoCD Application CR. If charts use `fullnameOverride` or a different naming
convention, add `app.kubernetes.io/name` as a pod label and extract it via the collector
instead.

### Annotation-based overrides

Per-app behavior can be overridden via annotations on the ArgoCD `Application` CR:

| Annotation | Values | Effect |
|---|---|---|
| `kri.io/rightsizing` | `enabled`, `disabled` | Override global rightsizing for this app |
| `kri.io/diagnosis` | `enabled`, `disabled` | Override global diagnosis for this app |
| `kri.io/values-path` | e.g. `app.resources` | Non-standard Helm values key path for resource block |
| `kri.io/last-diagnosed` | RFC3339 timestamp | Set by operator; used for cooldown tracking |
| `kri.io/last-rightsized` | RFC3339 timestamp | Set by operator; records last rightsizing run |
| `kri.io/last-error` | RFC3339 timestamp | Set by operator on pipeline failure; used for backoff |

---

## Phase 1 — Rightsizing Operator

**Goal:** Automate the existing `kri inspect → kri plan → kri apply` workflow on a schedule, without any new external integrations.

### Scope

- Operator scaffolding: `controller-runtime` manager, leader election, health endpoints
- `RightsizingController` reconcile loop on configurable schedule
- Full inspect pipeline reused from CLI: ArgoCD app list, Prometheus metrics, behavioral classification, HPA validation, recommendations
- Plan generation and GitHub PR creation per app (reuses `pkg/plan`, `pkg/gitops`, `pkg/github`)
- Per-app `kri.io/last-rightsized` annotation written on successful PR creation or "no findings" run
- Per-app `kri.io/last-error` annotation written on pipeline failure; used to enforce backoff before retry
- `kri.io/rightsizing: disabled` annotation respected
- `ignore_apps` and `ignore_namespaces` config respected
- Operator deployed as a `Deployment` with a `ConfigMap` for config and `Secret` for credentials
- Helm chart under `tools/kri-operator/chart/`
- CLI commands unchanged; CLI and operator share all `pkg/` logic

### What Phase 1 does not include

- Diagnosis / Coralogix / Slack
- Per-app values key path mapping (`kri.io/values-path`)
- Multi-source ArgoCD Application support
- Metrics or observability for the operator itself

### Reconcile loop — RightsizingController

```
on schedule tick:
  list all Application CRs in argocd_namespace
  filter: skip ignore_apps, ignore_namespaces, kri.io/rightsizing=disabled
  filter: skip apps where kri.io/last-rightsized within requeue_interval
  filter: skip apps where kri.io/last-error within backoff window
  for each app:
    run inspect pipeline (existing buildRows logic)
    if pipeline error:
      annotate app: kri.io/last-error = now
      log error, continue to next app
    if recommendations exist:
      build plan
      push values-resources.yaml branch
      ensure PR
      annotate app: kri.io/last-rightsized = now
    else:
      annotate app: kri.io/last-rightsized = now
```

The backoff window for `kri.io/last-error` should start at 1h and double on repeated
failures, capped at `requeue_interval`. This prevents a broken app (e.g. Prometheus
unreachable for one cluster) from being retried on every tick.

### Deliverables

- `tools/kri-operator/main.go`
- `tools/kri-operator/controller/rightsizing.go`
- `tools/kri-operator/chart/` — Helm chart
- `tools/kri-operator/README.md`
- Updated `pkg/config` to support operator runtime fields

---

## Phase 2 — Rollback Diagnosis

**Goal:** When an ArgoCD application degrades and rolls back due to a failed deployment, automatically surface the root cause from Coralogix logs and notify the team in Slack — without anyone having to manually `kubectl logs` or dig through the log aggregator.

### Problem statement

When a new image is deployed and the application fails (probe failure driven by application error), ArgoCD degrades and rolls back. The developer sees a rollback notification but has no immediate context on what went wrong. They must manually correlate the rollout timestamp, find the failed pods, query logs, and identify the error — a process that is slow, requires cluster access, and relies on logs not having been GC'd.

### Scope

- `DiagnosisController` watching `Application` CRDs for `Degraded` health transitions
- New-transition detection: on operator startup, controller-runtime issues an initial LIST for all Applications; any already-`Degraded` apps must not be re-diagnosed if `kri.io/last-diagnosed` is already set. On startup with no existing annotations, suppress diagnosis for apps already in `Degraded` state by writing `kri.io/last-diagnosed = now` without querying Coralogix or posting to Slack.
- Rollout window extraction from `status.operationState` (`startedAt`, `finishedAt`, failed revision SHA)
- Failed pod identification via ReplicaSet history walk (pod → ReplicaSet → Deployment chain); kube-state-metrics Prometheus series used as fallback if pods are already GC'd. Note: if both pods and kube-state-metrics series are gone (aggressive GC + short Prometheus retention), the diagnosis report must surface this explicitly rather than silently producing an empty result.
- Kubernetes Events API query for probe failure events (`Unhealthy` reason, `involvedObject` matching failed pods) — used as anchor timestamp for log window
- Coralogix log query for the diagnosis window:
  - If `app_name_field` is configured: query `resource.attributes.k8s_deployment_name == <app>`
  - If `app_name_field` is empty: query by `resource.attributes.k8s_pod_name IN [pod1, pod2, ...]`
  - Filtered to configured `severity_filter` severities
  - Bounded to `[rollout_startedAt, probe_failure_timestamp + log_window_seconds]`
- Error grouping: deduplicate identical stack traces, surface first occurrence of each unique error
- Slack notification via Block Kit (see Notification Format below)
- Cooldown enforcement via `kri.io/last-diagnosed` annotation — suppress re-diagnosis within `diagnosis_cooldown`
- `kri.io/diagnosis: disabled` annotation respected

### What Phase 2 does not include

- Resource correlation (Prometheus metrics at failure time) — deferred to Phase 3
- Automatic remediation or PR creation
- PagerDuty or other notification channels

### Reconcile loop — DiagnosisController

```
on Application status change:
  if health.status != Degraded: return
  if previous health.status == Degraded: return  # already degraded, not a new transition
  if kri.io/diagnosis == disabled: return
  if kri.io/last-diagnosed within diagnosis_cooldown: return

  # Startup suppression: first time we see a Degraded app with no annotation,
  # mark it without diagnosing to avoid spurious notifications on operator restart.
  if kri.io/last-diagnosed not set AND operator uptime < 60s:
    annotate app: kri.io/last-diagnosed = now
    return

  extract rollout window from status.operationState
  walk ReplicaSet history → identify pods from failed revision
  query Kubernetes Events for Unhealthy events on those pods → get probe_failure_timestamp
  query Coralogix for error logs in diagnosis window
  group errors by message/stack trace, sort by first occurrence
  assemble DiagnosisReport
  post to Slack
  annotate app: kri.io/last-diagnosed = now
```

### DiagnosisReport structure

```go
type DiagnosisReport struct {
    App              string
    Cluster          string
    Namespace        string
    FailedRevision   string
    RolloutStartedAt time.Time
    ProbeFailedAt    time.Time
    Containers       []ContainerDiagnosis
    DataGap          string    // non-empty if pod or log data was unavailable; included in Slack message
}

type ContainerDiagnosis struct {
    Name        string
    ProbeType   string // readiness, liveness
    ErrorGroups []ErrorGroup
}

type ErrorGroup struct {
    Message       string
    Count         int
    FirstSeenAt   time.Time
    LastSeenAt    time.Time
    SampleLogLine string
}
```

### Notification format

Slack Block Kit message:

```
🔴  Rollback: myapp  (production / in-cluster)

Revision a3f9c1d  failed after 4m 12s
Readiness probe failed at 14:32:18 UTC

Container: api
  [ERROR] NullPointerException at UserService.java:147       ×14  first: 14:32:01
  [ERROR] Failed to connect to postgres://db:5432/app        ×3   first: 14:31:58

→ View logs in Coralogix  |  View app in ArgoCD
```

Key design decisions:
- First unique error by timestamp, not most frequent — the cause comes before the cascade
- Link directly to a pre-built Coralogix query for the diagnosis window so the developer can drill in without constructing the query manually
- Link to the ArgoCD app UI for rollback status and history
- No log line reproduction beyond a single sample per error group — keeps the message scannable and avoids Slack message size limits
- If `DataGap` is set, include a warning block: "⚠️ Pod data unavailable — log results may be incomplete"
- Notification posted after a configurable `diagnosis_delay_seconds` (default: 30s) to allow Alertmanager to fire first, keeping the Slack thread coherent

### New packages

**`pkg/coralogix`**

The query format depends on the Coralogix API endpoint in use (DataPrime vs Lucene-based
logs API). Verify the exact query syntax and authentication header shape against a live
Coralogix instance before implementing. The interface below is stable regardless of
which endpoint backs it.

```go
type Client struct { /* endpoint, api key, http client */ }

func NewClient(endpoint, apiKey string) *Client

// QueryLogs returns log entries matching the filter within the time window.
// Attributes use the resource.attributes.k8s_* naming convention.
// If podNames is non-empty, queries by resource.attributes.k8s_pod_name
// regardless of appNameField.
func (c *Client) QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, error)

type LogQuery struct {
    AppNameField string    // e.g. "resource.attributes.k8s_deployment_name"; empty = use PodNames
    AppName      string
    PodNames     []string
    Severities   []string
    From         time.Time
    To           time.Time
}

type LogEntry struct {
    Timestamp  time.Time
    Severity   string
    Message    string
    Container  string
    Pod        string
    Attributes map[string]string
}
```

**`pkg/slack`**

```go
type Client struct { /* webhook url, http client */ }

func NewClient(webhookURL string) *Client
func (c *Client) PostDiagnosis(ctx context.Context, r diagnosis.DiagnosisReport) error
func (c *Client) PostRightsizingError(ctx context.Context, app string, err error) error
```

**`pkg/diagnosis`**

`pkg/diagnosis` imports `pkg/coralogix` for `LogEntry` and `LogQuery`.
`pkg/coralogix` must not import `pkg/diagnosis` — the dependency is one-directional.

```go
func BuildReport(
    app argo.App,
    events []corev1.Event,
    logs []coralogix.LogEntry,
) DiagnosisReport

func GroupErrors(logs []coralogix.LogEntry) []ErrorGroup
func CoralogixQueryURL(endpoint string, q coralogix.LogQuery) string
```

### Deliverables

- `tools/kri-operator/controller/diagnosis.go`
- `pkg/coralogix/client.go`
- `pkg/slack/client.go`
- `pkg/diagnosis/report.go`
- Updated `pkg/config` for Coralogix and Slack fields
- Updated operator Helm chart with new secret mounts
- Updated `tools/kri-operator/README.md`

---

## Phase 3 — Resource Correlation at Failure Time

**Goal:** Enrich the diagnosis report with resource utilization context from Prometheus at the time of the failed rollout, to distinguish application errors from resource pressure failures.

### Scope

- Query Prometheus for CPU and memory usage from the failed pods during the rollout window using a `UsageAt(time.Time)` variant of the existing metrics client
- Detect resource-pressure signatures: mem p99 approaching limit (`MemLimitRatio ≥ 0.9`), CPU throttle rate spike
- Add resource context block to the Slack notification when resource pressure is detected:

```
Container: api
  Memory: 487Mi / 512Mi limit (95%) at time of failure  ⚠️  possible OOM contributor
  CPU throttle: 34% in final 2 minutes
```

- When no resource pressure detected: omit the block entirely (keeps the notification clean for the common case of pure application errors)
- `kri.io/values-path` annotation support — allows per-app override of the Helm values key path for charts that don't use the standard `resources:` top-level key

### Deliverables

- `UsageAt` method on `pkg/metrics.Client`
- Resource pressure detection in `pkg/diagnosis`
- Updated `pkg/slack` notification template
- `kri.io/values-path` annotation handling in `pkg/gitops`

---

## Phase 4 — Multi-Source ArgoCD and Operator Maturity

**Goal:** Production hardening and support for multi-source ArgoCD Applications.

### Scope

- Multi-source Application support: detect `spec.sources[]`, identify the `ref: values` source, extract its `repoURL` as the write target for `values-resources.yaml`. Adds `ValuesRepoURL` to `pkg/argo.App`. Until Phase 4 lands, apps using `spec.sources[]` are skipped with a logged warning (consistent with AR-01 fix in kri CLI).
- Operator observability: Prometheus metrics exposed on `/metrics`
  - `kri_rightsizing_runs_total` — counter by app, result (`pr_opened`, `no_findings`, `error`)
  - `kri_diagnosis_runs_total` — counter by app, result (`notified`, `cooldown`, `error`)
  - `kri_apps_watched_total` — gauge
- Structured logging via `log/slog` replacing `fmt.Fprintf(os.Stderr, ...)`
- Configurable `ignore_apps` and `ignore_namespaces` via both config file and `kri.io/` annotations
- OCI and Chartmuseum repo support in hvc (separate tool, same phase milestone)
- End-to-end integration test scaffold using `envtest`

### Deliverables

- Multi-source support in `pkg/argo`
- `pkg/operator/metrics.go` — Prometheus instrumentation
- `log/slog` migration across all packages
- `tools/kri-operator/test/` — envtest integration tests
- hvc OCI registry support

---

## Open Questions

**Coralogix API endpoint and query syntax** — The attribute naming convention is confirmed (`resource.attributes.k8s_*` with underscores). Before implementing `pkg/coralogix`, verify the query syntax and authentication header shape against a live Coralogix instance. DataPrime and the Lucene-based logs API have different query formats; the choice determines the implementation of `QueryLogs`. Run a manual query against a known deployment name to confirm `resource.attributes.k8s_deployment_name` resolves correctly before writing any client code.

**ArgoCD app name vs Deployment name alignment** — `app_name_field` queries by `resource.attributes.k8s_deployment_name`. Verify that Deployment names in managed clusters match `spec.destination.name` on their ArgoCD Application CRs. If they diverge due to `fullnameOverride` or chart naming conventions, add `app.kubernetes.io/name` as a pod label and extract it via the collector instead.

**Multi-source apps in scope for Phase 1 rollout** — Check which current apps use `spec.sources[]` before deploying Phase 1. Those apps will be skipped with a warning until Phase 4. If any high-priority apps are multi-source, Phase 4 may need to be pulled forward.

**Diagnosis cooldown storage under RBAC constraints** — `kri.io/last-diagnosed` requires write access to `Application` CRs. If RBAC prevents this, fall back to an in-cluster `ConfigMap` keyed by app name. Determine RBAC posture before Phase 2 implementation.

**Slack message threading** — The operator will post a diagnosis message ~30 seconds after the rollback event. If Alertmanager posts a degradation alert to the same channel, there will be two independent messages. The 30-second delay makes the kri message appear second, which is semantically correct (alert = "it's down", diagnosis = "here's why"), but threading the diagnosis as a reply to the Alertmanager alert would be cleaner. This requires Alertmanager webhook integration to find the parent message timestamp — defer unless the two-message pattern proves noisy in practice.
