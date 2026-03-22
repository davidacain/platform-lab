package plan

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"

	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/output"
	"gopkg.in/yaml.v3"
)

// ContainerPlan holds current state, observed metrics, and recommended values
// for a single container.
type ContainerPlan struct {
	Name string `yaml:"name"`

	// Current configured values
	CurrentCPURequest string `yaml:"current_cpu_request"`
	CurrentCPULimit   string `yaml:"current_cpu_limit"`
	CurrentMemRequest string `yaml:"current_mem_request"`
	CurrentMemLimit   string `yaml:"current_mem_limit"`

	// Observed usage
	CPUP95     string `yaml:"cpu_p95"`
	CPUP99     string `yaml:"cpu_p99"`
	MemP95     string `yaml:"mem_p95"`
	MemP99     string `yaml:"mem_p99"`
	MemLimPct  string `yaml:"mem_lim_pct"`
	Behavior   string `yaml:"behavior"`
	Confidence string `yaml:"confidence"`

	// Recommended values
	CPURequest string `yaml:"cpu_request"`
	CPULimit   string `yaml:"cpu_limit"`
	MemRequest string `yaml:"mem_request"`
	MemLimit   string `yaml:"mem_limit"`
}

// AppPlan holds the plan for a single ArgoCD application.
type AppPlan struct {
	App        string          `yaml:"app"`
	Repo       string          `yaml:"repo"`
	ValuesFile string          `yaml:"values_file"` // repo-relative path to values-resources.yaml
	Window     string          `yaml:"window"`
	Apply      bool            `yaml:"apply"`
	Containers []ContainerPlan `yaml:"containers"`
}

// Build converts inspect rows into a list of AppPlans, one per app.
// Only rows with actionable (non-hold, non-tolerance) recommendations are included.
// Multiple pods of the same app+container are deduplicated — first occurrence wins.
func Build(rows []output.PodRow, window string) []AppPlan {
	appIndex := map[string]int{}
	seen := map[string]string{} // dedupKey → recommendation text for divergence detection
	var plans []AppPlan

	for _, r := range rows {
		if !r.Recommendation.IsActionable {
			continue
		}
		if r.Recommendation.Resources == nil {
			continue
		}

		dedupKey := r.AppName + "/" + r.Container
		if stored, ok := seen[dedupKey]; ok {
			if r.Recommendation.Text != stored {
				fmt.Fprintf(os.Stderr, "warn: %s: divergent recommendations across pods (keeping first): %q vs %q\n",
					dedupKey, stored, r.Recommendation.Text)
			}
			continue
		}
		seen[dedupKey] = r.Recommendation.Text

		idx, exists := appIndex[r.AppName]
		if !exists {
			plans = append(plans, AppPlan{
				App:        r.AppName,
				Repo:       r.RepoURL,
				ValuesFile: path.Join(r.ChartPath, "values-resources.yaml"),
				Window:     window,
				Apply:      true,
			})
			idx = len(plans) - 1
			appIndex[r.AppName] = idx
		}

		res := r.Recommendation.Resources
		plans[idx].Containers = append(plans[idx].Containers, ContainerPlan{
			Name:              r.Container,
			CurrentCPURequest: r.CPUReq.String(),
			CurrentCPULimit:   r.CPULim.String(),
			CurrentMemRequest: r.MemReq.String(),
			CurrentMemLimit:   r.MemLim.String(),
			CPUP95:            fmtCPU(r.CPUP95, r.HasData),
			CPUP99:            fmtCPU(r.CPUP99, r.HasData),
			MemP95:            fmtMem(r.MemP95, r.HasData),
			MemP99:            fmtMem(r.MemP99, r.HasData),
			MemLimPct:         fmtMemLimPct(r.MemP99, r.MemLim.Value()),
			Behavior:          string(r.Behavior),
			Confidence:        fmt.Sprintf("%.0f%%", r.Confidence*100),
			CPURequest:        res.CPURequest,
			CPULimit:          res.CPULimit,
			MemRequest:        res.MemRequest,
			MemLimit:          res.MemLimit,
		})
	}

	return plans
}

// Write serialises plans to kri-plan.yaml in dir (current directory if empty).
func Write(plans []AppPlan, dir string) error {
	if len(plans) == 0 {
		fmt.Println("No actionable recommendations — nothing to plan.")
		return nil
	}

	p := planPath(dir)
	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("create %s: %w", p, err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(plans); err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}

	fmt.Printf("Wrote %s with %d app(s). Edit apply: false to skip an app, then run: kri apply\n", p, len(plans))
	return nil
}

// Read deserialises kri-plan.yaml from dir (current directory if empty).
func Read(dir string) ([]AppPlan, error) {
	p := planPath(dir)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	var plans []AppPlan
	if err := yaml.Unmarshal(data, &plans); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	for i, ap := range plans {
		if ap.App == "" {
			return nil, fmt.Errorf("%s: entry %d missing required field 'app'", p, i)
		}
		if len(ap.Containers) == 0 {
			return nil, fmt.Errorf("%s: app %q has no containers", p, ap.App)
		}
	}
	return plans, nil
}

func planPath(dir string) string {
	if dir == "" {
		return "kri-plan.yaml"
	}
	return filepath.Join(dir, "kri-plan.yaml")
}

func fmtCPU(cores float64, hasData bool) string {
	if !hasData {
		return "-"
	}
	milli := int64(math.Round(cores * 1000))
	return fmt.Sprintf("%dm", milli)
}

func fmtMem(bytes float64, hasData bool) string {
	if !hasData {
		return "-"
	}
	return fmt.Sprintf("%.1fMi", bytes/1048576)
}

func fmtMemLimPct(memP99 float64, limBytes int64) string {
	if memP99 == 0 || limBytes == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", (memP99/float64(limBytes))*100)
}
