# platform-lab

Personal platform engineering lab for experimenting with Kubernetes tooling,
Helm charts, and cloud-native infrastructure patterns.

## Stack

- **Runtime**: Node.js 22 on Alpine
- **Local Kubernetes**: k3d (k3s in Docker) on WSL2
- **Package management**: Helm
- **Target cloud**: GCP (GKE Autopilot)
- **Container runtime**: Docker Desktop with WSL2 integration
- **IaC**: OpenTofu with GCS backend (planned)

## Repo structure
```
demo-app/            # Minimal Node.js HTTP server
  server.js
  package.json
  Dockerfile
  namespace.yaml     # Kubernetes namespace with Pod Security Standards
  chart/             # Helm chart for Kubernetes deployment
    Chart.yaml
    values.yaml
    templates/
docs/                # Runbooks and how-to guides
pkg/                 # Shared Go packages
  version/           # Semver utilities
tools/
  helm-version-check/  # hvc — compares deployed Helm releases to upstream
  pod-security-inspector/  # psi — audits pod security, mesh, and network policy
  k8s-resource-inspector/  # kri — right-sizes workloads and opens PRs with recommendations
  kri-operator/      # kri-operator — runs kri on a schedule as a Kubernetes operator
```

## General conventions

- No hardcoded secrets — use environment variables or secret references
- MIT licensed
- Keep things simple — this is a lab, not production

@CLAUDE-go.md
@CLAUDE-helm.md
@CLAUDE-private.md
