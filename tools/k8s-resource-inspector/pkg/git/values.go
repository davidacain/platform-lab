package git

import (
	"fmt"
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ContainerValues holds the resource configuration for one container as declared
// in a Helm values file.
type ContainerValues struct {
	Name       string
	CPURequest resource.Quantity
	CPULimit   resource.Quantity
	MemRequest resource.Quantity
	MemLimit   resource.Quantity
}

// ValuesConfig is the result of reading a Helm values file from git.
type ValuesConfig struct {
	RepoURL    string
	FilePath   string // repo-relative path, e.g. "demo-app/chart/values.yaml"
	Containers []ContainerValues
}

// ReadValues shallow-clones repoURL at the given revision, reads the values file
// at chartPath/valuesFile, and returns the resource configuration per container.
//
// revision may be a branch name, tag, or "HEAD".
// chartPath is the chart directory within the repo (e.g. "demo-app/chart").
// valuesFile is the filename within chartPath (e.g. "values.yaml").
func ReadValues(repoURL, revision, chartPath, valuesFile string) (*ValuesConfig, error) {
	tmpDir, err := os.MkdirTemp("", "kri-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneOpts := &gogit.CloneOptions{
		URL:          repoURL,
		Depth:        1,
		SingleBranch: true,
	}
	// Non-HEAD revisions: treat as a branch reference.
	if revision != "" && revision != "HEAD" {
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(revision)
	}

	if _, err := gogit.PlainClone(tmpDir, false, cloneOpts); err != nil {
		return nil, fmt.Errorf("clone %s: %w", repoURL, err)
	}

	repoRelPath := filepath.Join(chartPath, valuesFile)
	absPath := filepath.Join(tmpDir, repoRelPath)

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", repoRelPath, err)
	}

	containers, err := parseContainerResources(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", repoRelPath, err)
	}

	return &ValuesConfig{
		RepoURL:    repoURL,
		FilePath:   repoRelPath,
		Containers: containers,
	}, nil
}

// parseContainerResources extracts resource requests/limits from a Helm values file.
// Supports two patterns:
//   - containers[].resources  (multi-container chart, e.g. demo-app)
//   - resources               (single-container chart at top level)
func parseContainerResources(data []byte) ([]ContainerValues, error) {
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, err
	}

	// Multi-container pattern: containers[].resources
	if rawList, ok := values["containers"].([]interface{}); ok {
		var result []ContainerValues
		for _, raw := range rawList {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			cv := ContainerValues{Name: name}
			applyResources(&cv, m)
			result = append(result, cv)
		}
		return result, nil
	}

	// Single-container pattern: top-level resources key.
	cv := ContainerValues{Name: "app"}
	applyResources(&cv, values)
	if !cv.CPURequest.IsZero() || !cv.MemRequest.IsZero() {
		return []ContainerValues{cv}, nil
	}

	return nil, nil
}

func applyResources(cv *ContainerValues, m map[string]interface{}) {
	res, ok := m["resources"].(map[string]interface{})
	if !ok {
		return
	}

	if reqs, ok := res["requests"].(map[string]interface{}); ok {
		cv.CPURequest = parseQty(reqs["cpu"])
		cv.MemRequest = parseQty(reqs["memory"])
	}
	if lims, ok := res["limits"].(map[string]interface{}); ok {
		cv.CPULimit = parseQty(lims["cpu"])
		cv.MemLimit = parseQty(lims["memory"])
	}
}

func parseQty(v interface{}) resource.Quantity {
	s, ok := v.(string)
	if !ok {
		return resource.Quantity{}
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.Quantity{}
	}
	return q
}
