# Building the demo-app Image

## Prerequisites

- Docker Desktop with WSL2 integration enabled

## Build

From the repo root:

```bash
docker build -t demo-app:1.0.0 demo-app/
```

The tag `1.0.0` matches the `appVersion` in [demo-app/chart/Chart.yaml](../demo-app/chart/Chart.yaml). Update both together when cutting a new version.

## Load into k3d

After building, import the image into your local cluster so Kubernetes can pull it:

```bash
k3d image import demo-app:1.0.0 -c platform-lab
```

Verify the import:

```bash
kubectl get nodes  # cluster is reachable
docker exec k3d-platform-lab-server-0 crictl images | grep demo-app
```

## Deploy

```bash
helm upgrade --install demo-app demo-app/chart/
```
