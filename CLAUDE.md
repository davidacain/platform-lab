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
```

## Conventions

### Docker
- Base image: `node:22-alpine`
- Always run as non-root (`node` user, uid 1000)
- Use `tini` as PID 1 for correct signal handling
- Apply `apk upgrade --no-cache` at build time for OS patch coverage

### Kubernetes / Helm
- Resource requests == limits (Guaranteed QoS) to prevent node overcommit
- SecurityContext: drop all capabilities, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`
- PDB enabled when replicaCount > 1 or autoscaling is configured
- Features are off by default — activated by providing configuration (e.g. non-empty `ingress.hosts`, `autoscaling.maxReplicas`, `configMap.data`) rather than boolean flags
- Mutually exclusive config options (HPA metrics, PDB policy) are validated with `fail` in templates

### General
- No hardcoded secrets — use environment variables or secret references
- MIT licensed
- Keep things simple — this is a lab, not production
