package plan

import (
	"fmt"
	"os"
	"path"

	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/output"
	"gopkg.in/yaml.v3"
)

// ContainerPlan holds the desired resource state for a single container.
type ContainerPlan struct {
	Name       string `yaml:"name"`
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
	Apply      bool            `yaml:"apply"`
	Containers []ContainerPlan `yaml:"containers"`
}

// Build converts inspect rows into a list of AppPlans, one per app.
// Only rows with actionable (non-hold, non-tolerance) recommendations are included.
// Multiple pods of the same app+container are deduplicated — first occurrence wins.
func Build(rows []output.PodRow) []AppPlan {
	type appKey = string
	type containerKey = string

	appIndex := map[appKey]int{}      // app name -> index in result
	seen := map[string]bool{}         // "app/container" dedup key
	var plans []AppPlan

	for _, r := range rows {
		if r.Recommendation.Hold || r.Recommendation.Text == "" || r.Recommendation.Text == "within tolerance" {
			continue
		}
		if r.Recommendation.Resources == nil {
			continue
		}

		dedupKey := r.AppName + "/" + r.Container
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		idx, exists := appIndex[r.AppName]
		if !exists {
			valuesFile := path.Join(r.ChartPath, "values-resources.yaml")
			plans = append(plans, AppPlan{
				App:        r.AppName,
				Repo:       r.RepoURL,
				ValuesFile: valuesFile,
				Apply:      true,
			})
			idx = len(plans) - 1
			appIndex[r.AppName] = idx
		}

		res := r.Recommendation.Resources
		plans[idx].Containers = append(plans[idx].Containers, ContainerPlan{
			Name:       r.Container,
			CPURequest: res.CPURequest,
			CPULimit:   res.CPULimit,
			MemRequest: res.MemRequest,
			MemLimit:   res.MemLimit,
		})
	}

	return plans
}

// Write serialises plans to kri-plan.yaml in the current directory.
func Write(plans []AppPlan) error {
	if len(plans) == 0 {
		fmt.Println("No actionable recommendations — nothing to plan.")
		return nil
	}

	f, err := os.Create("kri-plan.yaml")
	if err != nil {
		return fmt.Errorf("create kri-plan.yaml: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(plans); err != nil {
		return fmt.Errorf("write kri-plan.yaml: %w", err)
	}

	fmt.Printf("Wrote kri-plan.yaml with %d app(s). Edit apply: false to skip an app, then run: kri apply\n", len(plans))
	return nil
}

// Read deserialises kri-plan.yaml from the current directory.
func Read() ([]AppPlan, error) {
	data, err := os.ReadFile("kri-plan.yaml")
	if err != nil {
		return nil, fmt.Errorf("read kri-plan.yaml: %w", err)
	}
	var plans []AppPlan
	if err := yaml.Unmarshal(data, &plans); err != nil {
		return nil, fmt.Errorf("parse kri-plan.yaml: %w", err)
	}
	return plans, nil
}
