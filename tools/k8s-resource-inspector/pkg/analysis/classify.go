package analysis

import (
	"math"

	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/metrics"
	"k8s.io/apimachinery/pkg/api/resource"
)

// BehaviorClass classifies a workload's resource usage pattern.
type BehaviorClass string

const (
	BehaviorStatic  BehaviorClass = "STATIC"  // low variance, consistent usage
	BehaviorSpiky   BehaviorClass = "SPIKY"   // high variance between p50 and p99
	BehaviorGrowth  BehaviorClass = "GROWTH"  // consistent upward memory trend
	BehaviorRunaway BehaviorClass = "RUNAWAY" // p99 at or near memory limit
	BehaviorMixed   BehaviorClass = "MIXED"   // pods within workload disagree
	BehaviorUnknown BehaviorClass = "UNKNOWN" // insufficient data
)

// ClassifyPod classifies a single container's behavior from its usage metrics.
// Returns the behavior class and a confidence score (0.0–1.0).
func ClassifyPod(u metrics.Usage, memLim resource.Quantity) (BehaviorClass, float64) {
	if !u.HasData {
		return BehaviorUnknown, 0
	}

	// RUNAWAY: p99 within 10% of or exceeding memory limit — highest priority.
	memLimBytes := float64(memLim.Value())
	if memLimBytes > 0 && u.MemP99 > 0 {
		if u.MemP99/memLimBytes >= 0.9 {
			return BehaviorRunaway, 0.95
		}
	}

	cpuRatio := safeRatio(u.CPUP99, u.CPUP50)
	memRatio := safeRatio(u.MemP99, u.MemP50)

	// SPIKY: high p99/p50 variance.
	if cpuRatio >= 2.0 || memRatio >= 1.8 {
		return BehaviorSpiky, 0.85
	}

	// GROWTH: memory trend > 1% of p50 per hour.
	trendThreshold := u.MemP50 * 0.01
	if u.MemP50 > 0 && u.MemTrend > trendThreshold {
		return BehaviorGrowth, 0.75
	}

	// STATIC: low variance and flat trend.
	trendFlat := u.MemP50 == 0 || math.Abs(u.MemTrend) <= trendThreshold
	if cpuRatio < 1.5 && memRatio < 1.3 && trendFlat {
		return BehaviorStatic, 0.9
	}

	return BehaviorUnknown, 0
}

// ClassifyWorkload aggregates pod-level behaviors into a workload-level class.
// Returns MIXED (with a breakdown map) when pods disagree.
func ClassifyWorkload(behaviors []BehaviorClass) (BehaviorClass, map[BehaviorClass]int) {
	counts := make(map[BehaviorClass]int)
	for _, b := range behaviors {
		counts[b]++
	}
	if len(counts) == 1 {
		for b := range counts {
			return b, nil
		}
	}
	return BehaviorMixed, counts
}

// safeRatio returns p99/p50, or 1.0 when p50 is zero (no variance data).
func safeRatio(p99, p50 float64) float64 {
	if p50 <= 0 {
		return 1.0
	}
	return p99 / p50
}
