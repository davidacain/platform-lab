package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// jsonRow is the serialization-friendly version of PodRow for --output json.
type jsonRow struct {
	App       string `json:"app"`
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`

	// Inventory
	CPURequest string `json:"cpu_request"`
	CPULimit   string `json:"cpu_limit"`
	MemRequest string `json:"mem_request"`
	MemLimit   string `json:"mem_limit"`

	// Usage
	CPUP95Cores float64 `json:"cpu_p95_cores"`
	CPUP99Cores float64 `json:"cpu_p99_cores"`
	MemP95Mi    float64 `json:"mem_p95_mi"`
	MemP99Mi    float64 `json:"mem_p99_mi"`
	MemLimRatio float64 `json:"mem_lim_ratio"`
	MemTrend    float64 `json:"mem_trend_mi_per_hour"`
	HasData     bool    `json:"has_data"`

	// Classification
	Behavior   string  `json:"behavior"`
	Confidence float64 `json:"confidence"`

	// HPA
	HPA jsonHPA `json:"hpa"`

	// Recommendation
	Recommendation jsonRec `json:"recommendation"`

	// Git
	ValuesFile string `json:"values_file,omitempty"`
}

type jsonHPA struct {
	Status         string        `json:"status"`
	Findings       []jsonFinding  `json:"findings,omitempty"`
	Recommendation *jsonHPARec   `json:"recommendation,omitempty"`
}

type jsonHPARec struct {
	Text        string `json:"text"`
	Driver      string `json:"driver"`
	MinReplicas int32  `json:"min_replicas"`
	TargetCPU   *int32 `json:"target_cpu_pct,omitempty"`
	TargetMem   *int32 `json:"target_memory_pct,omitempty"`
	Reason      string `json:"reason"`
}

type jsonFinding struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type jsonRec struct {
	Hold       bool   `json:"hold"`
	HoldReason string `json:"hold_reason,omitempty"`
	Text       string `json:"text,omitempty"`
}

func PrintJSON(rows []PodRow) error {
	out := make([]jsonRow, 0, len(rows))

	for _, r := range rows {
		memLimRat := 0.0
		if r.MemP99 > 0 && !r.MemLim.IsZero() {
			memLimRat = r.MemP99 / float64(r.MemLim.Value())
		}

		jr := jsonRow{
			App:         r.AppName,
			Cluster:     r.Cluster,
			Namespace:   r.Namespace,
			Pod:         r.PodName,
			Container:   r.Container,
			CPURequest:  qOrDash(r.CPUReq),
			CPULimit:    qOrDash(r.CPULim),
			MemRequest:  qOrDash(r.MemReq),
			MemLimit:    qOrDash(r.MemLim),
			CPUP95Cores: r.CPUP95,
			CPUP99Cores: r.CPUP99,
			MemP95Mi:    r.MemP95 / 1048576,
			MemP99Mi:    r.MemP99 / 1048576,
			MemLimRatio: memLimRat,
			MemTrend:    r.MemTrend / 1048576,
			HasData:     r.HasData,
			Behavior:    string(r.Behavior),
			Confidence:  r.Confidence,
			HPA: jsonHPA{
				Status: r.HPAStatus.Status,
			},
			Recommendation: jsonRec{
				Hold:       r.Recommendation.Hold,
				HoldReason: r.Recommendation.HoldReason,
				Text:       r.Recommendation.Text,
			},
			ValuesFile: r.ValuesFilePath,
		}

		for _, f := range r.HPAStatus.Findings {
			jr.HPA.Findings = append(jr.HPA.Findings, jsonFinding{
				Severity: f.Severity,
				Message:  f.Message,
			})
		}

		if r.HPARecommendation != nil {
			jr.HPA.Recommendation = &jsonHPARec{
				Text:        r.HPARecommendation.Text,
				Driver:      r.HPARecommendation.Driver,
				MinReplicas: r.HPARecommendation.MinReplicas,
				TargetCPU:   r.HPARecommendation.TargetCPU,
				TargetMem:   r.HPARecommendation.TargetMemory,
				Reason:      r.HPARecommendation.Reason,
			}
		}

		out = append(out, jr)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
