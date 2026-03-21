# hvc auth test environment

Runs a ChartMuseum instance in your k3d cluster with basic auth enabled,
giving you a real authenticated Helm repo to test against.

## Deploy

```bash
# Add the ChartMuseum chart repo
helm repo add chartmuseum https://chartmuseum.github.io/charts
helm repo update

# Deploy to k3d (NodePort 30800)
kubectl create namespace helm-repos --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install chartmuseum chartmuseum/chartmuseum \
  -n helm-repos \
  -f tools/helm-version-check/testenv/chartmuseum-values.yaml

# Verify it's up
kubectl rollout status deployment/chartmuseum -n helm-repos
```

## Push a test chart

```bash
# Package the dummy chart
helm package tools/helm-version-check/testenv/test-chart

# Push to ChartMuseum (basic auth)
curl -u testuser:testpass \
  --data-binary "@test-chart-1.2.0.tgz" \
  http://localhost:30800/api/charts

# Confirm it's in the index
curl -u testuser:testpass http://localhost:30800/index.yaml
```

> Port 30800 is the NodePort. On k3d with default networking this is
> reachable at localhost:30800 from WSL2.

## Configure hvc

`~/.hvc/config.yaml`:

```yaml
repos:
  - name: local-test
    type: helm-http
    url: http://localhost:30800
    auth:
      type: basic
      username: testuser
      password: testpass
```

Then run:

```bash
hvc check
```

You should see `test-chart` listed (assuming a Helm release is deployed that
uses it), or test the index fetch directly:

```bash
curl -u testuser:testpass http://localhost:30800/index.yaml | grep test-chart
```

## Tear down

```bash
helm uninstall chartmuseum -n helm-repos
kubectl delete namespace helm-repos
```
