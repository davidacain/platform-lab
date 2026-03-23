# kri-operator

Kubernetes operator that automates the `kri inspect → plan → apply` workflow on a schedule. Shares all core packages with the kri CLI.

## Overview

kri-operator runs as a Deployment in your cluster and periodically inspects ArgoCD Application workloads for resource rightsizing opportunities. When actionable recommendations are found, it opens GitHub PRs with `values-resources.yaml` changes — the same output as `kri apply`.

The kri CLI remains fully functional as a developer and debug interface into the same underlying logic.

## Phase 1 scope

- Schedule-based rightsizing loop (configurable interval, default 6h)
- Full inspect pipeline: ArgoCD app list, Prometheus metrics, behavioral classification, HPA validation, recommendations
- GitHub PR creation per app (same as `kri apply`)
- Per-app `kri.io/last-rightsized` annotation on success
- Per-app `kri.io/last-error` and `kri.io/error-count` annotations with exponential backoff on failure
- `kri.io/rightsizing: disabled` annotation and `ignore_apps`/`ignore_namespaces` config respected
- Leader election (safe to run with multiple replicas)

## Installation

Build the image:

```bash
docker build -t kri-operator:0.1.0 -f tools/kri-operator/Dockerfile .
```

Deploy via Helm:

```bash
helm upgrade --install kri-operator tools/kri-operator/chart/ \
  -n kri-operator --create-namespace \
  --set githubToken="$GITHUB_TOKEN" \
  --set-file config.yaml=~/.kri/config.yaml
```

## Configuration

The operator reads the same `~/.kri/config.yaml` format as the CLI, mounted as a ConfigMap at `/etc/kri/config.yaml`. The `operator:` section controls operator-specific behaviour:

```yaml
clusters:
  - argocd_cluster: in-cluster
    prometheus: http://prometheus.monitoring.svc:9090

argocd_namespace: argocd

git:
  author_name: kri
  author_email: kri@noreply.local

github:
  base_branch: main

operator:
  requeue_interval: 6h       # how often to run the inspection loop
  diagnosis_cooldown: 1h     # minimum time between runs for the same app on error
  ignore_apps: []            # ArgoCD app names to skip
  ignore_namespaces: []      # namespaces to skip
```

`GITHUB_TOKEN` must be set in the environment (via the `githubToken` Helm value or an external secret).

## Annotation-based overrides

Annotate an ArgoCD `Application` CR to control per-app behaviour:

| Annotation | Values | Effect |
|---|---|---|
| `kri.io/rightsizing` | `enabled`, `disabled` | Override global rightsizing for this app |
| `kri.io/last-rightsized` | RFC3339 timestamp | Set by operator on success; used to gate the next run |
| `kri.io/last-error` | RFC3339 timestamp | Set by operator on failure; drives exponential backoff |
| `kri.io/error-count` | integer | Tracks consecutive failures; backoff = 1h × 2^(count-1), capped at requeue_interval |

## RBAC

The operator requires cluster-scoped permissions:

- `argoproj.io/applications`: `get`, `list`, `patch` (to list apps and write annotations)
- `autoscaling/horizontalpodautoscalers`: `get`, `list` (for HPA validation)
- `coordination.k8s.io/leases`: full access (leader election)
- `events`: `create`, `patch` (controller-runtime event recorder)

The Helm chart creates a `ClusterRole` and `ClusterRoleBinding` for the operator's `ServiceAccount`.

## Health endpoints

| Path | Port | Purpose |
|------|------|---------|
| `/healthz` | 8081 | Liveness probe |
| `/readyz` | 8081 | Readiness probe |

## Exponential backoff

On pipeline failure for an app, the operator sets `kri.io/last-error` and increments `kri.io/error-count`. The retry window is:

```
backoff = 1h × 2^(error-count - 1), capped at requeue_interval
```

Example for a 6h requeue interval:
- 1st failure: retry after 1h
- 2nd failure: retry after 2h
- 3rd failure: retry after 4h
- 4th+ failure: retry after 6h (capped)

On success, both annotations are cleared.

## ArgoCD setup

See the kri CLI [Apply workflow](../k8s-resource-inspector/README.md#apply-workflow) for the one-time AppSet change needed to pick up `values-resources.yaml`.

## Roadmap

See [docs/kri-operator.md](../../docs/kri-operator.md) for the full product specification including Phase 2 (rollback diagnosis) and beyond.
