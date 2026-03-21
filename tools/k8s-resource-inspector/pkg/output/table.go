package output

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/analysis"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/hpa"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PodRow is one row in the output table.
type PodRow struct {
	AppName   string
	Cluster   string
	Namespace string
	PodName   string
	Container string

	// Inventory (from kube-state-metrics)
	CPUReq resource.Quantity
	CPULim resource.Quantity
	MemReq resource.Quantity
	MemLim resource.Quantity

	// Usage percentiles (from Prometheus)
	CPUP50  float64
	CPUP95  float64
	CPUP99  float64
	MemP50  float64
	MemP95  float64
	MemP99  float64
	MemTrend float64 // bytes/hour
	HasData  bool

	// Classification (Phase 3)
	Behavior   analysis.BehaviorClass
	Confidence float64

	// HPA validation (Phase 4)
	HPAStatus hpa.Validation

	// Recommendation (Phase 5)
	Recommendation analysis.Recommendation

	// Values file path from git (Phase 6)
	ValuesFilePath string
}

func PrintTable(rows []PodRow) error {
	if len(rows) == 0 {
		fmt.Println("No pods found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "APP\tCLUSTER\tNAMESPACE\tPOD\tCONTAINER\tCPU_REQ\tCPU_P95\tCPU_P99\tMEM_REQ\tMEM_P95\tMEM_P99\tMEM/LIM\tBEHAVIOR\tCONF\tHPA\tREC")

	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.AppName,
			r.Cluster,
			r.Namespace,
			r.PodName,
			r.Container,
			qOrDash(r.CPUReq),
			coresOrDash(r.CPUP95, r.HasData),
			coresOrDash(r.CPUP99, r.HasData),
			qOrDash(r.MemReq),
			bytesOrDash(r.MemP95, r.HasData),
			bytesOrDash(r.MemP99, r.HasData),
			memLimRatio(r.MemP99, r.MemLim),
			behaviorOrDash(r.Behavior),
			confOrDash(r.Confidence),
			hpaStatus(r.HPAStatus),
			recText(r.Recommendation, r.ValuesFilePath),
		)
	}

	return w.Flush()
}

func qOrDash(q resource.Quantity) string {
	if q.IsZero() {
		return "-"
	}
	return q.String()
}

func coresOrDash(cores float64, hasData bool) string {
	if !hasData {
		return "-"
	}
	milli := int64(cores * 1000)
	return resource.NewMilliQuantity(milli, resource.DecimalSI).String()
}

func bytesOrDash(bytes float64, hasData bool) string {
	if !hasData {
		return "-"
	}
	return fmt.Sprintf("%.1fMi", bytes/1048576)
}

func memLimRatio(memP99 float64, memLim resource.Quantity) string {
	if memP99 == 0 || memLim.IsZero() {
		return "-"
	}
	limBytes := float64(memLim.Value())
	if limBytes == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", (memP99/limBytes)*100)
}

func behaviorOrDash(b analysis.BehaviorClass) string {
	if b == "" {
		return "-"
	}
	return string(b)
}

func confOrDash(c float64) string {
	if c == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", c*100)
}

func recText(r analysis.Recommendation, valuesFile string) string {
	if r.Hold {
		if r.HoldReason != "" {
			return "hold: " + r.HoldReason
		}
		return "hold"
	}
	if r.Text == "" {
		return "-"
	}
	if valuesFile != "" {
		return fmt.Sprintf("%s  [%s]", r.Text, valuesFile)
	}
	return r.Text
}

func hpaStatus(v hpa.Validation) string {
	switch v.Status {
	case "NONE", "":
		return "-"
	case "OK":
		return "OK"
	default:
		if len(v.Findings) > 0 {
			return fmt.Sprintf("%s: %s", v.Status, v.Findings[0].Message)
		}
		return v.Status
	}
}
