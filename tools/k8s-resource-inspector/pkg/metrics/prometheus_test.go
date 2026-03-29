package metrics

import (
	"context"
	"testing"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// mockAPI implements just enough of v1.API for PressureAt tests.
type mockAPI struct {
	v1.API // embed to satisfy the interface; unimplemented methods panic

	// responses is a list of vectors returned in order for successive Query calls.
	responses []model.Vector
	calls     int
}

func (m *mockAPI) Query(_ context.Context, _ string, _ time.Time, _ ...v1.Option) (model.Value, v1.Warnings, error) {
	if m.calls >= len(m.responses) {
		return model.Vector{}, nil, nil
	}
	v := m.responses[m.calls]
	m.calls++
	return v, nil, nil
}

func metric(labels map[string]string) model.Metric {
	m := model.Metric{}
	for k, v := range labels {
		m[model.LabelName(k)] = model.LabelValue(v)
	}
	return m
}

func TestPressureAt_NoPods(t *testing.T) {
	c := &Client{api: &mockAPI{}}
	got, err := c.PressureAt(context.Background(), "ns", nil, time.Now().Add(-time.Minute), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil map for empty pod list, got %v", got)
	}
}

func TestPressureAt_HasMemPressure(t *testing.T) {
	// Simulate a container at 95% memory utilisation (above 0.9 threshold).
	memPeak := model.SampleValue(950e6)   // 950 MiB
	memLimit := model.SampleValue(1000e6) // 1000 MiB → ratio 0.95
	cpuThrottle := model.SampleValue(0.1) // 10% → below threshold

	mock := &mockAPI{
		responses: []model.Vector{
			// query 0: mem peak
			{{Metric: metric(map[string]string{"container": "app", "pod": "app-abc", "namespace": "ns"}), Value: memPeak}},
			// query 1: mem limit
			{{Metric: metric(map[string]string{"container": "app", "pod": "app-abc", "namespace": "ns"}), Value: memLimit}},
			// query 2: cpu throttle
			{{Metric: metric(map[string]string{"container": "app", "pod": "app-abc", "namespace": "ns"}), Value: cpuThrottle}},
		},
	}
	c := &Client{api: mock}
	now := time.Now()
	result, err := c.PressureAt(context.Background(), "ns", []string{"app-abc"}, now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, ok := result["app"]
	if !ok {
		t.Fatal("expected entry for container 'app'")
	}
	if !p.HasData {
		t.Error("HasData should be true")
	}
	if !p.HasMemPressure {
		t.Errorf("HasMemPressure should be true; MemLimitRatio=%.3f", p.MemLimitRatio)
	}
	if p.HasCPUThrottle {
		t.Errorf("HasCPUThrottle should be false; CPUThrottleRatio=%.3f", p.CPUThrottleRatio)
	}
}

func TestPressureAt_HasCPUThrottle(t *testing.T) {
	memPeak := model.SampleValue(100e6)  // well under any limit
	memLimit := model.SampleValue(500e6) // ratio 0.2 → no mem pressure
	cpuThrottle := model.SampleValue(0.4) // 40% → above 0.25 threshold

	mock := &mockAPI{
		responses: []model.Vector{
			{{Metric: metric(map[string]string{"container": "worker", "pod": "worker-xyz", "namespace": "ns"}), Value: memPeak}},
			{{Metric: metric(map[string]string{"container": "worker", "pod": "worker-xyz", "namespace": "ns"}), Value: memLimit}},
			{{Metric: metric(map[string]string{"container": "worker", "pod": "worker-xyz", "namespace": "ns"}), Value: cpuThrottle}},
		},
	}
	c := &Client{api: mock}
	now := time.Now()
	result, err := c.PressureAt(context.Background(), "ns", []string{"worker-xyz"}, now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := result["worker"]
	if p.HasMemPressure {
		t.Errorf("HasMemPressure should be false; ratio=%.3f", p.MemLimitRatio)
	}
	if !p.HasCPUThrottle {
		t.Errorf("HasCPUThrottle should be true; ratio=%.3f", p.CPUThrottleRatio)
	}
}

func TestPressureAt_NoLimit(t *testing.T) {
	// Container with no memory limit set (limit=0) → ratio should stay 0.
	mock := &mockAPI{
		responses: []model.Vector{
			{{Metric: metric(map[string]string{"container": "app", "pod": "app-1", "namespace": "ns"}), Value: 800e6}},
			{}, // no limit metric
			{}, // no throttle metric
		},
	}
	c := &Client{api: mock}
	now := time.Now()
	result, err := c.PressureAt(context.Background(), "ns", []string{"app-1"}, now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := result["app"]
	if p.MemLimitRatio != 0 {
		t.Errorf("MemLimitRatio should be 0 when no limit; got %.3f", p.MemLimitRatio)
	}
	if p.HasMemPressure {
		t.Error("HasMemPressure should be false when no limit")
	}
}
