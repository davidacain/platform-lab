CLUSTER := platform-lab

.PHONY: build test cluster-up cluster-down deps clean

## build: build all CLI tools
build:
	go build ./tools/...

## test: run all tests
test:
	go test ./...

## cluster-up: create a local k3d cluster
cluster-up:
	k3d cluster create $(CLUSTER)

## cluster-down: delete the local k3d cluster
cluster-down:
	k3d cluster delete $(CLUSTER)

## deps: install ArgoCD, Prometheus, and demo-app into the cluster
deps:
	helm repo add argo https://argoproj.github.io/argo-helm
	helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	helm repo update
	kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install argocd argo/argo-cd \
	  -n argocd -f tools/k8s-resource-inspector/testenv/argocd-values.yaml --wait
	kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install prometheus prometheus-community/prometheus \
	  -n monitoring -f tools/k8s-resource-inspector/testenv/prometheus-values.yaml --wait
	kubectl apply -f tools/k8s-resource-inspector/testenv/demo-app-argocd.yaml

## dev: full local dev setup — cluster + deps
dev: cluster-up deps

## clean: tear down the local cluster
clean: cluster-down
