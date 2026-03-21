# demo-app

Minimal Node.js HTTP server for Kubernetes experimentation. Returns JSON with status, hostname, and timestamp — useful for verifying routing, load balancing, and rolling deployments.

## Application

`server.js` uses only Node's built-in `http` module. No runtime dependencies.

```
GET /  →  { "status": "ok", "hostname": "...", "timestamp": "..." }
```

Listens on `PORT` (default `8080`). Handles `SIGTERM` for graceful shutdown.

## Container

```bash
docker build -t demo-app:1.0.0 .
docker run --rm -p 8080:8080 demo-app:1.0.0
```

The image runs as the non-root `node` user (uid 1000) with `tini` as PID 1.

## Helm chart

### Install

```bash
helm upgrade --install demo-app ./chart -n demo-app
```

### Common overrides

```bash
# Scale out with a PDB
helm upgrade --install demo-app ./chart -n demo-app \
  --set replicaCount=3

# Enable ingress
helm upgrade --install demo-app ./chart -n demo-app \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=demo-app.local \
  --set ingress.hosts[0].paths[0].path=/

# Enable HPA
helm upgrade --install demo-app ./chart -n demo-app \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10
```

### Chart features

| Feature | Default | Enabled by |
|---------|---------|------------|
| Ingress | off | `ingress.enabled=true` |
| HPA | off | `autoscaling.enabled=true` |
| PDB | off | `replicaCount > 1` |
| ConfigMap env injection | off | `configMap.enabled=true` |
| Service Account | on | `serviceAccount.create=false` to disable |

### Security defaults

- `runAsNonRoot: true`, `runAsUser: 1000`
- `readOnlyRootFilesystem: true`
- `allowPrivilegeEscalation: false`
- `capabilities.drop: [ALL]`
- Namespace enforces `baseline` Pod Security Standard (`namespace.yaml`)

## Local k3d workflow

```bash
# Build and import into the cluster
docker build -t demo-app:1.0.0 .
k3d image import demo-app:1.0.0 -c platform-lab

# Apply namespace then deploy
kubectl apply -f namespace.yaml
helm upgrade --install demo-app ./chart -n demo-app

# Verify
kubectl rollout status deployment/demo-app -n demo-app
kubectl port-forward svc/demo-app 8080:80 -n demo-app
curl localhost:8080
```
