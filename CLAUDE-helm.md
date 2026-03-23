# Helm and Kubernetes conventions

## Docker
- Base image: `node:22-alpine`
- Always run as non-root (`node` user, uid 1000)
- Use `tini` as PID 1 for correct signal handling
- Apply `apk upgrade --no-cache` at build time for OS patch coverage

## Kubernetes / Helm
- Resource requests == limits (Guaranteed QoS) to prevent node overcommit
- SecurityContext: drop all capabilities, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`
- PDB enabled when replicaCount > 1 or autoscaling is configured
- Features are off by default — activated by providing configuration (e.g. non-empty `ingress.hosts`, `autoscaling.maxReplicas`, `configMap.data`) rather than boolean flags
- Mutually exclusive config options (HPA metrics, PDB policy) are validated with `fail` in templates
