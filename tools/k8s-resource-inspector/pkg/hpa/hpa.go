package hpa

import (
	"context"
	"fmt"

	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/analysis"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var hpaGVR = schema.GroupVersionResource{
	Group:    "autoscaling",
	Version:  "v2",
	Resource: "horizontalpodautoscalers",
}

// Info holds the fields from an HPA that are relevant to kri validation.
type Info struct {
	Name        string
	Namespace   string
	TargetKind  string
	TargetName  string // scaleTargetRef.name — used to join to workloads
	MinReplicas int32
	MaxReplicas int32
	CPUTarget   *int32 // averageUtilization %, nil if HPA does not target CPU
	MemTarget   *int32 // averageUtilization %, nil if HPA does not target memory
}

// Finding is a single HPA validation result.
type Finding struct {
	Severity string // ERROR or WARN
	Message  string
}

// Validation is the aggregated result of all HPA checks for a workload.
type Validation struct {
	Status   string    // OK, WARN, ERROR, NONE (no HPA present)
	Findings []Finding
}

// List returns all HPAs in the given namespace.
func List(ctx context.Context, dynClient dynamic.Interface, namespace string) ([]Info, error) {
	list, err := dynClient.Resource(hpaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list HPAs in %q: %w", namespace, err)
	}

	var infos []Info
	for _, item := range list.Items {
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec == nil {
			continue
		}

		ref, _ := spec["scaleTargetRef"].(map[string]interface{})
		targetKind, _ := ref["kind"].(string)
		targetName, _ := ref["name"].(string)

		minReplicas := int32(1)
		if v, ok := spec["minReplicas"].(int64); ok {
			minReplicas = int32(v)
		}
		maxReplicas := int32(0)
		if v, ok := spec["maxReplicas"].(int64); ok {
			maxReplicas = int32(v)
		}

		info := Info{
			Name:        item.GetName(),
			Namespace:   item.GetNamespace(),
			TargetKind:  targetKind,
			TargetName:  targetName,
			MinReplicas: minReplicas,
			MaxReplicas: maxReplicas,
		}

		metricsList, _ := spec["metrics"].([]interface{})
		for _, m := range metricsList {
			metric, _ := m.(map[string]interface{})
			if metric["type"] != "Resource" {
				continue
			}
			res, _ := metric["resource"].(map[string]interface{})
			name, _ := res["name"].(string)
			target, _ := res["target"].(map[string]interface{})
			if target["type"] != "Utilization" {
				continue
			}
			pct, _ := target["averageUtilization"].(int64)
			pct32 := int32(pct)
			switch name {
			case "cpu":
				info.CPUTarget = &pct32
			case "memory":
				info.MemTarget = &pct32
			}
		}

		infos = append(infos, info)
	}

	return infos, nil
}

// FindForTarget returns the HPA whose scaleTargetRef.name matches targetName, or nil.
func FindForTarget(hpas []Info, targetName string) *Info {
	for i := range hpas {
		if hpas[i].TargetName == targetName {
			return &hpas[i]
		}
	}
	return nil
}

// Validate runs all HPA checks for a single container against its usage metrics.
func Validate(h *Info, u metrics.Usage, cpuReq, memReq resource.Quantity, behavior analysis.BehaviorClass) Validation {
	if h == nil {
		return Validation{Status: "NONE"}
	}

	var findings []Finding

	// CPU request missing — HPA cannot function without a request to compute % against.
	if h.CPUTarget != nil && cpuReq.IsZero() {
		findings = append(findings, Finding{"ERROR", "HPA targets CPU but no CPU request is set"})
	}

	// Memory request missing.
	if h.MemTarget != nil && memReq.IsZero() {
		findings = append(findings, Finding{"ERROR", "HPA targets memory but no memory request is set"})
	}

	// CPU utilization checks — only meaningful when we have usage data.
	if h.CPUTarget != nil && u.HasData && !cpuReq.IsZero() {
		cpuReqCores := float64(cpuReq.MilliValue()) / 1000.0
		if cpuReqCores > 0 {
			p95Pct := (u.CPUP95 / cpuReqCores) * 100
			p50Pct := (u.CPUP50 / cpuReqCores) * 100
			target := float64(*h.CPUTarget)

			// p95 already exceeds target → HPA is perpetually scaling out.
			if p95Pct > target {
				findings = append(findings, Finding{"WARN", fmt.Sprintf(
					"CPU p95 utilization (%.0f%%) exceeds HPA target (%d%%) — HPA may be perpetually scaling",
					p95Pct, *h.CPUTarget,
				)})
			}

			// p50 well below target → HPA will likely never trigger.
			if p50Pct > 0 && p50Pct < target*0.3 {
				findings = append(findings, Finding{"WARN", fmt.Sprintf(
					"CPU HPA target (%d%%) is far above p50 utilization (%.0f%%) — HPA may never trigger",
					*h.CPUTarget, p50Pct,
				)})
			}
		}
	}

	// minReplicas=1 on a SPIKY workload — cold starts will absorb spikes poorly.
	if h.MinReplicas == 1 && behavior == analysis.BehaviorSpiky {
		findings = append(findings, Finding{"WARN",
			"minReplicas=1 on SPIKY workload — consider raising to absorb traffic spikes"})
	}

	// Scaling metric mismatch: CPU-only HPA on a memory-trending workload.
	if h.CPUTarget != nil && h.MemTarget == nil {
		if behavior == analysis.BehaviorGrowth || behavior == analysis.BehaviorRunaway {
			findings = append(findings, Finding{"WARN",
				"CPU-only HPA on a memory-trending workload — consider adding a memory metric"})
		}
	}

	if len(findings) == 0 {
		return Validation{Status: "OK"}
	}

	status := "WARN"
	for _, f := range findings {
		if f.Severity == "ERROR" {
			status = "ERROR"
			break
		}
	}
	return Validation{Status: status, Findings: findings}
}
