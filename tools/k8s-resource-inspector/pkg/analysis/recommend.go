package analysis

import (
	"fmt"
	"math"
	"strings"

	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Recommendation holds a rightsizing suggestion for a single container.
type Recommendation struct {
	Text       string // human-readable; empty when held below confidence threshold
	Hold       bool
	HoldReason string
}

// Recommend generates a recommendation based on behavior, confidence, and usage metrics.
// threshold is the minimum confidence required to emit a recommendation (0.0–1.0).
func Recommend(behavior BehaviorClass, confidence, threshold float64, u metrics.Usage, cpuReq, memReq, memLim resource.Quantity) Recommendation {
	if !u.HasData {
		return Recommendation{Hold: true, HoldReason: "no data"}
	}

	if confidence < threshold {
		return Recommendation{Hold: true, HoldReason: fmt.Sprintf("confidence %.0f%% below threshold %.0f%%", confidence*100, threshold*100)}
	}

	switch behavior {
	case BehaviorStatic:
		return recommendStatic(u, cpuReq, memReq)
	case BehaviorRunaway:
		return recommendRunaway(u, memLim)
	case BehaviorSpiky:
		return Recommendation{Hold: true, HoldReason: "SPIKY — monitor before rightsizing"}
	case BehaviorGrowth:
		return Recommendation{Hold: true, HoldReason: "GROWTH — still trending"}
	case BehaviorMixed:
		return Recommendation{Hold: true, HoldReason: "MIXED — investigate pod divergence first"}
	default:
		return Recommendation{Hold: true, HoldReason: "UNKNOWN — insufficient data"}
	}
}

// recommendStatic suggests reducing requests toward p99 + headroom.
func recommendStatic(u metrics.Usage, cpuReq, memReq resource.Quantity) Recommendation {
	var parts []string

	// CPU: p99 + 20% headroom, rounded up to nearest 10m, minimum 10m.
	recCPUMillis := int64(math.Ceil(u.CPUP99 * 1200)) // cores * 1000m/core * 1.2 headroom
	recCPUMillis = roundUpTo(recCPUMillis, 10)
	if recCPUMillis < 10 {
		recCPUMillis = 10
	}
	if significantDiff(float64(recCPUMillis), float64(cpuReq.MilliValue())) {
		parts = append(parts, fmt.Sprintf("CPU %s→%dm", cpuReq.String(), recCPUMillis))
	}

	// Memory: p99 + 30% headroom, rounded up to nearest Mi, minimum 16Mi.
	if u.MemP99 > 0 {
		recMemMi := int64(math.Ceil(u.MemP99 * 1.3 / 1048576))
		if recMemMi < 16 {
			recMemMi = 16
		}
		curMemMi := memReq.Value() / 1048576
		if significantDiff(float64(recMemMi), float64(curMemMi)) {
			parts = append(parts, fmt.Sprintf("MEM %s→%dMi", memReq.String(), recMemMi))
		}
	}

	if len(parts) == 0 {
		return Recommendation{Text: "within tolerance"}
	}
	return Recommendation{Text: strings.Join(parts, ", ")}
}

// recommendRunaway suggests increasing the memory limit to give headroom above p99.
func recommendRunaway(u metrics.Usage, memLim resource.Quantity) Recommendation {
	if u.MemP99 <= 0 || memLim.IsZero() {
		return Recommendation{Hold: true, HoldReason: "RUNAWAY — insufficient data"}
	}
	recMi := int64(math.Ceil(u.MemP99 * 1.5 / 1048576))
	return Recommendation{Text: fmt.Sprintf("Increase MEM limit to %dMi (RUNAWAY)", recMi)}
}

// significantDiff returns true when recommended and current differ by more than 10%.
func significantDiff(recommended, current float64) bool {
	if current == 0 {
		return recommended > 0
	}
	return math.Abs(recommended-current)/current > 0.10
}

// roundUpTo rounds v up to the nearest multiple of step.
func roundUpTo(v, step int64) int64 {
	if step == 0 {
		return v
	}
	return ((v + step - 1) / step) * step
}
