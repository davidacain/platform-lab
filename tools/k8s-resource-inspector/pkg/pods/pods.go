package pods

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Pod holds the inventory data for a single running container.
type Pod struct {
	Namespace     string
	PodName       string
	ContainerName string
	NodeName      string
	CPURequest    resource.Quantity
	CPULimit      resource.Quantity
	MemRequest    resource.Quantity
	MemLimit      resource.Quantity
}

// PodLister can list running pods in a namespace.
type PodLister interface {
	ListPods(ctx context.Context, namespace string) ([]Pod, error)
}

// PromLister implements PodLister backed by Prometheus kube-state-metrics.
type PromLister struct {
	api v1.API
}

func NewPromLister(prometheusURL string) (*PromLister, error) {
	client, err := api.NewClient(api.Config{Address: prometheusURL})
	if err != nil {
		return nil, fmt.Errorf("create prometheus client: %w", err)
	}
	return &PromLister{api: v1.NewAPI(client)}, nil
}

// ListPods returns all running pods in the namespace with their resource configuration.
func (p *PromLister) ListPods(ctx context.Context, namespace string) ([]Pod, error) {
	now := time.Now()

	type key struct{ namespace, pod, container string }
	pods := make(map[key]*Pod)

	// Only include pods in Running phase.
	runningFilter := fmt.Sprintf(`kube_pod_status_phase{namespace=%q,phase="Running"} == 1`, namespace)

	queries := []struct {
		name     string
		resource string
		kind     string // "requests" or "limits"
	}{
		{"cpu requests", "cpu", "requests"},
		{"cpu limits", "cpu", "limits"},
		{"memory requests", "memory", "requests"},
		{"memory limits", "memory", "limits"},
	}

	type rawResult struct {
		resource string
		kind     string
		vec      model.Vector
	}
	results := make([]rawResult, 0, len(queries))

	for _, q := range queries {
		query := fmt.Sprintf(
			`kube_pod_container_resource_%s{namespace=%q,resource=%q,container!=""} * on(namespace,pod) group_left() (%s)`,
			q.kind, namespace, q.resource, runningFilter,
		)
		vec, err := p.queryVector(ctx, query, now)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", q.name, err)
		}
		results = append(results, rawResult{q.resource, q.kind, vec})
	}

	// Populate pods map from query results.
	for _, r := range results {
		for _, s := range r.vec {
			k := key{
				namespace: string(s.Metric["namespace"]),
				pod:       string(s.Metric["pod"]),
				container: string(s.Metric["container"]),
			}
			if _, ok := pods[k]; !ok {
				pods[k] = &Pod{
					Namespace:     k.namespace,
					PodName:       k.pod,
					ContainerName: k.container,
				}
			}

			val := float64(s.Value)
			switch {
			case r.resource == "cpu" && r.kind == "requests":
				pods[k].CPURequest = *resource.NewMilliQuantity(int64(val*1000), resource.DecimalSI)
			case r.resource == "cpu" && r.kind == "limits":
				pods[k].CPULimit = *resource.NewMilliQuantity(int64(val*1000), resource.DecimalSI)
			case r.resource == "memory" && r.kind == "requests":
				pods[k].MemRequest = *resource.NewQuantity(int64(val), resource.BinarySI)
			case r.resource == "memory" && r.kind == "limits":
				pods[k].MemLimit = *resource.NewQuantity(int64(val), resource.BinarySI)
			}
		}
	}

	// Enrich with node name from kube_pod_info.
	nodeVec, err := p.queryVector(ctx, fmt.Sprintf(`kube_pod_info{namespace=%q}`, namespace), now)
	if err != nil {
		return nil, fmt.Errorf("query pod info: %w", err)
	}
	nodeMap := make(map[string]string) // pod → node
	for _, s := range nodeVec {
		nodeMap[string(s.Metric["pod"])] = string(s.Metric["node"])
	}
	for k, pod := range pods {
		pod.NodeName = nodeMap[k.pod]
	}

	// Return as a flat slice. Order is non-deterministic (map iteration); Phase 7 will sort.
	out := make([]Pod, 0, len(pods))
	for _, pod := range pods {
		out = append(out, *pod)
	}
	return out, nil
}

func (p *PromLister) queryVector(ctx context.Context, query string, ts time.Time) (model.Vector, error) {
	result, _, err := p.api.Query(ctx, query, ts)
	if err != nil {
		return nil, err
	}
	vec, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected vector result, got %T", result)
	}
	return vec, nil
}
