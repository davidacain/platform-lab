# platform-lab

Personal platform engineering lab for experimenting with Kubernetes tooling, Helm charts, and cloud-native infrastructure patterns.

## Stack

| Layer | Technology |
|-------|------------|
| Runtime | Node.js 22 on Alpine |
| Local Kubernetes | k3d (k3s in Docker) on WSL2 |
| Package management | Helm |
| Target cloud | GCP (GKE Autopilot) |
| Container runtime | Docker Desktop with WSL2 integration |
| IaC | OpenTofu with GCS backend *(planned)* |

## Repo structure

```
demo-app/        # Minimal Node.js HTTP server
  server.js      # Application entry point
  package.json
  Dockerfile
  namespace.yaml # Kubernetes namespace with Pod Security Standards
  chart/         # Helm chart
    Chart.yaml
    values.yaml
    templates/
docs/            # Runbooks and how-to guides
pkg/             # Shared Go packages (client, version utilities)
tools/
  helm-version-check/      # hvc — compare deployed releases to upstream versions
  pod-security-inspector/  # psi — audit pod security contexts, mesh, and netpol
  k8s-resource-inspector/  # kri — right-size workloads and open PRs with recommendations
  kri-operator/            # kri-operator — runs kri on a schedule as a Kubernetes operator
```

## Quick start

### Prerequisites

- Docker Desktop with WSL2 integration
- [k3d](https://k3d.io/) — `brew install k3d` or see k3d docs
- [Helm](https://helm.sh/) — `brew install helm`

### Create a local cluster

```bash
k3d cluster create platform-lab
```

### Build and deploy demo-app

```bash
# Build the image
docker build -t demo-app:1.0.0 demo-app/

# Load it into the k3d cluster
k3d image import demo-app:1.0.0 -c platform-lab

# Create the namespace
kubectl apply -f demo-app/namespace.yaml

# Deploy via Helm
helm upgrade --install demo-app demo-app/chart/ -n demo-app
```

See [docs/build.md](docs/build.md) for the full build and deployment guide.

## Design decisions

**Guaranteed QoS** — resource requests equal limits on all workloads to prevent node overcommit and ensure predictable scheduling.

**Secure by default** — containers run as non-root (`node` uid 1000), with a read-only root filesystem, all Linux capabilities dropped, and `allowPrivilegeEscalation: false`. The namespace enforces the `baseline` Pod Security Standard.

**tini as PID 1** — ensures correct signal forwarding and zombie process reaping without a full init system.

**Features off by default** — ingress, autoscaling, and ConfigMap are all disabled in `values.yaml` and enabled via Helm overrides, keeping the default footprint minimal.

## License

MIT
