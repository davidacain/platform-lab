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

// queryOrder matches the order queries are issued in PressureAt:
// 0: mem peak, 1: mem request, 2: mem limit, 3: cpu usage, 4: cpu throttle
func responses(memPeak, memRequest, memLimit, cpuCores, cpuThrottle float64, pod, container string) []model.Vector {
	labels := map[string]string{"container": container, "pod": pod, "namespace": "ns"}
	sv := func(v float64) model.Vector {
		if v < 0 {
			return model.Vector{} // sentinel: no data for this query
		}
		return model.Vector{{Metric: metric(labels), Value: model.SampleValue(v)}}
	}
	return []model.Vector{sv(memPeak), sv(memRequest), sv(memLimit), sv(cpuCores), sv(cpuThrottle)}
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
	// peak (600 MiB) exceeds request (512 MiB) → HasMemPressure
	mock := &mockAPI{responses: responses(600e6, 512e6, 1024e6, 0.3, 0.0, "app-abc", "app")}
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
		t.Errorf("HasMemPressure should be true; peak=%.0f request=%.0f", p.MemPeakBytes, p.MemRequestBytes)
	}
	if p.HasCPUThrottle {
		t.Errorf("HasCPUThrottle should be false; throttle=%.3f", p.CPUThrottleRatio)
	}
	if p.CPUCores != 0.3 {
		t.Errorf("CPUCores = %.3f, want 0.3", p.CPUCores)
	}
}

func TestPressureAt_HasCPUThrottle(t *testing.T) {
	// any throttle > 0 → HasCPUThrottle; mem under request → no mem pressure
	mock := &mockAPI{responses: responses(100e6, 512e6, 1024e6, 0.05, 0.02, "worker-xyz", "worker")}
	c := &Client{api: mock}
	now := time.Now()
	result, err := c.PressureAt(context.Background(), "ns", []string{"worker-xyz"}, now.Add(-5*time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := result["worker"]
	if p.HasMemPressure {
		t.Errorf("HasMemPressure should be false; peak=%.0f request=%.0f", p.MemPeakBytes, p.MemRequestBytes)
	}
	if !p.HasCPUThrottle {
		t.Errorf("HasCPUThrottle should be true for any throttle > 0; got %.4f", p.CPUThrottleRatio)
	}
}

func TestPressureAt_NoRequest(t *testing.T) {
	// No memory request set → HasMemPressure stays false even if peak is high.
	// -1 sentinel means no data returned for that query.
	mock := &mockAPI{responses: responses(800e6, -1, -1, 0.1, 0.0, "app-1", "app")}
	c := &Client{api: mock}
	now := time.Now()
	result, err := c.PressureAt(context.Background(), "ns", []string{"app-1"}, now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := result["app"]
	if p.MemRequestBytes != 0 {
		t.Errorf("MemRequestBytes should be 0 when not set; got %.0f", p.MemRequestBytes)
	}
	if p.HasMemPressure {
		t.Error("HasMemPressure should be false when no request is set")
	}
}

func TestPressureAt_BelowRequest(t *testing.T) {
	// Peak well under request → no pressure.
	mock := &mockAPI{responses: responses(200e6, 512e6, 1024e6, 0.2, 0.0, "app-1", "app")}
	c := &Client{api: mock}
	now := time.Now()
	result, err := c.PressureAt(context.Background(), "ns", []string{"app-1"}, now.Add(-time.Minute), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := result["app"]
	if p.HasMemPressure {
		t.Errorf("HasMemPressure should be false; peak=%.0f < request=%.0f", p.MemPeakBytes, p.MemRequestBytes)
	}
	if p.HasCPUThrottle {
		t.Error("HasCPUThrottle should be false; throttle=0")
	}
}
