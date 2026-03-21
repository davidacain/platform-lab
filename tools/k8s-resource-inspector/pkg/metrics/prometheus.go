package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// ContainerKey uniquely identifies a container within a pod.
type ContainerKey struct {
	Namespace, Pod, Container string
}

// Usage holds Prometheus-derived usage metrics for a single container
// over an observation window.
type Usage struct {
	CPUP50 float64 // cores
	CPUP95 float64
	CPUP99 float64
	MemP50 float64 // bytes
	MemP95 float64
	MemP99    float64
	MemTrend  float64 // bytes/hour (positive = growing)
	HasData   bool    // false when window has insufficient samples
}

// Client wraps the Prometheus HTTP API for usage metric queries.
type Client struct {
	api v1.API
}

func NewClient(prometheusURL string) (*Client, error) {
	c, err := api.NewClient(api.Config{Address: prometheusURL})
	if err != nil {
		return nil, fmt.Errorf("create prometheus client: %w", err)
	}
	return &Client{api: v1.NewAPI(c)}, nil
}

// UsageMetrics queries CPU/memory percentiles and memory trend for all running
// containers in namespace over the observation window.
// window is a Prometheus duration string: "7d", "24h", "1h", etc.
func (c *Client) UsageMetrics(ctx context.Context, namespace, window string) (map[ContainerKey]Usage, error) {
	now := time.Now()
	// Subquery step — 5m is a reasonable granularity for a 30s scrape interval.
	step := "5m"

	type querySpec struct {
		name  string
		query string
		apply func(u *Usage, v float64)
	}

	queries := []querySpec{
		{
			name: "cpu p50",
			query: fmt.Sprintf(
				`quantile_over_time(0.50, rate(container_cpu_usage_seconds_total{namespace=%q,container!=""}[5m])[%s:%s])`,
				namespace, window, step,
			),
			apply: func(u *Usage, v float64) { u.CPUP50 = v },
		},
		{
			name: "cpu p95",
			query: fmt.Sprintf(
				`quantile_over_time(0.95, rate(container_cpu_usage_seconds_total{namespace=%q,container!=""}[5m])[%s:%s])`,
				namespace, window, step,
			),
			apply: func(u *Usage, v float64) { u.CPUP95 = v },
		},
		{
			name: "cpu p99",
			query: fmt.Sprintf(
				`quantile_over_time(0.99, rate(container_cpu_usage_seconds_total{namespace=%q,container!=""}[5m])[%s:%s])`,
				namespace, window, step,
			),
			apply: func(u *Usage, v float64) { u.CPUP99 = v },
		},
		{
			name: "mem p50",
			query: fmt.Sprintf(
				`quantile_over_time(0.50, container_memory_working_set_bytes{namespace=%q,container!=""}[%s:%s])`,
				namespace, window, step,
			),
			apply: func(u *Usage, v float64) { u.MemP50 = v },
		},
		{
			name: "mem p95",
			query: fmt.Sprintf(
				`quantile_over_time(0.95, container_memory_working_set_bytes{namespace=%q,container!=""}[%s:%s])`,
				namespace, window, step,
			),
			apply: func(u *Usage, v float64) { u.MemP95 = v },
		},
		{
			name: "mem p99",
			query: fmt.Sprintf(
				`quantile_over_time(0.99, container_memory_working_set_bytes{namespace=%q,container!=""}[%s:%s])`,
				namespace, window, step,
			),
			apply: func(u *Usage, v float64) { u.MemP99 = v },
		},
		{
			// deriv returns bytes/second — multiply by 3600 for bytes/hour.
			name: "mem trend",
			query: fmt.Sprintf(
				`deriv(container_memory_working_set_bytes{namespace=%q,container!=""}[%s]) * 3600`,
				namespace, window,
			),
			apply: func(u *Usage, v float64) { u.MemTrend = v },
		},
	}

	result := make(map[ContainerKey]Usage)

	for _, q := range queries {
		vec, err := c.queryVector(ctx, q.query, now)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", q.name, err)
		}
		for _, s := range vec {
			k := ContainerKey{
				Namespace: string(s.Metric["namespace"]),
				Pod:       string(s.Metric["pod"]),
				Container: string(s.Metric["container"]),
			}
			u := result[k]
			u.HasData = true
			q.apply(&u, float64(s.Value))
			result[k] = u
		}
	}

	return result, nil
}

func (c *Client) queryVector(ctx context.Context, query string, ts time.Time) (model.Vector, error) {
	result, _, err := c.api.Query(ctx, query, ts)
	if err != nil {
		return nil, err
	}
	vec, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected vector, got %T", result)
	}
	return vec, nil
}
