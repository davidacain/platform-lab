package inspect

import (
	"context"
	"fmt"
	"os"

	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/analysis"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/argo"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/config"
	gitvals "github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/git"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/hpa"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/metrics"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/output"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/pods"
	"k8s.io/client-go/dynamic"
)

// BuildRows runs the full inspect pipeline and returns one PodRow per container.
// Rows are unsorted and unfiltered — callers apply their own sort/filter.
func BuildRows(ctx context.Context, cfg *config.Config, dynClient dynamic.Interface, apps []argo.App, window string, confidenceThreshold float64) ([]output.PodRow, error) {
	minCPU := cfg.MinCPUMillis()
	minMem := cfg.MinMemoryMi()

	listerCache := map[string]pods.PodLister{}
	metricsCache := map[string]*metrics.Client{}

	type gitCacheKey struct{ repoURL, revision, valuesFile string }
	type gitCacheEntry struct{ config *gitvals.ValuesConfig }
	gitCache := map[gitCacheKey]*gitCacheEntry{}

	var rows []output.PodRow

	for _, app := range apps {
		promURL, ok := cfg.PrometheusFor(app.DestinationName)
		if !ok {
			fmt.Fprintf(os.Stderr, "warn: no Prometheus configured for cluster %q (app %s), skipping\n",
				app.DestinationName, app.Name)
			continue
		}

		lister, ok := listerCache[promURL]
		if !ok {
			var err error
			lister, err = pods.NewPromLister(promURL)
			if err != nil {
				return nil, fmt.Errorf("create pod lister for app %s: %w", app.Name, err)
			}
			listerCache[promURL] = lister
		}

		podList, err := lister.ListPods(ctx, app.Namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: list pods for app %s: %v\n", app.Name, err)
			continue
		}

		metricsClient, ok := metricsCache[promURL]
		if !ok {
			metricsClient, err = metrics.NewClient(promURL)
			if err != nil {
				return nil, fmt.Errorf("create metrics client for app %s: %w", app.Name, err)
			}
			metricsCache[promURL] = metricsClient
		}

		usageMap, err := metricsClient.UsageMetrics(ctx, app.Namespace, window)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: fetch usage metrics for app %s: %v\n", app.Name, err)
		}

		var gitConfig *gitvals.ValuesConfig
		if app.RepoURL != "" && len(app.ValueFiles) > 0 {
			gk := gitCacheKey{app.RepoURL, app.TargetRevision, app.ValueFiles[0]}
			if cached, ok := gitCache[gk]; ok {
				gitConfig = cached.config
			} else {
				gitConfig, err = gitvals.ReadValues(app.RepoURL, app.TargetRevision, app.Path, app.ValueFiles[0])
				if err != nil {
					fmt.Fprintf(os.Stderr, "warn: read git values for app %s: %v\n", app.Name, err)
				}
				gitCache[gk] = &gitCacheEntry{config: gitConfig}
			}
		}

		hpas, err := hpa.List(ctx, dynClient, app.Namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: list HPAs for app %s: %v\n", app.Name, err)
		}
		appHPA := hpa.FindForTarget(hpas, app.Name)

		type podWithBehavior struct {
			row      output.PodRow
			behavior analysis.BehaviorClass
		}
		var appPods []podWithBehavior

		for _, p := range podList {
			row := output.PodRow{
				AppName:      app.Name,
				Cluster:      app.DestinationName,
				Namespace:    p.Namespace,
				PodName:      p.PodName,
				WorkloadName: p.WorkloadName,
				Container:    p.ContainerName,
				CPUReq:       p.CPURequest,
				CPULim:       p.CPULimit,
				MemReq:       p.MemRequest,
				MemLim:       p.MemLimit,
				RepoURL:      app.RepoURL,
				ChartPath:    app.Path,
			}

			if u, ok := usageMap[metrics.ContainerKey{
				Namespace: p.Namespace,
				Pod:       p.PodName,
				Container: p.ContainerName,
			}]; ok {
				row.HasData = u.HasData
				row.CPUP50 = u.CPUP50
				row.CPUP95 = u.CPUP95
				row.CPUP99 = u.CPUP99
				row.MemP50 = u.MemP50
				row.MemP95 = u.MemP95
				row.MemP99 = u.MemP99
				row.MemTrend = u.MemTrend

				behavior, confidence := analysis.ClassifyPod(u, p.MemLimit)
				row.Behavior = behavior
				row.Confidence = confidence
				row.HPAStatus = hpa.Validate(appHPA, u, p.CPURequest, p.MemRequest, behavior)
				row.Recommendation = analysis.Recommend(behavior, confidence, confidenceThreshold, u, p.CPURequest, p.CPULimit, p.MemRequest, p.MemLimit, minCPU, minMem)

				if gitConfig != nil && !row.Recommendation.Hold && row.Recommendation.Text != "" {
					row.ValuesFilePath = gitConfig.FilePath
				}
			} else {
				row.Behavior = analysis.BehaviorUnknown
				row.HPAStatus = hpa.Validate(appHPA, metrics.Usage{}, p.CPURequest, p.MemRequest, analysis.BehaviorUnknown)
			}

			appPods = append(appPods, podWithBehavior{row: row, behavior: row.Behavior})
		}

		// Workload-level MIXED detection.
		if len(appPods) > 1 {
			behaviors := make([]analysis.BehaviorClass, len(appPods))
			for i, p := range appPods {
				behaviors[i] = p.behavior
			}
			workloadBehavior, _ := analysis.ClassifyWorkload(behaviors)
			if workloadBehavior == analysis.BehaviorMixed {
				for i := range appPods {
					appPods[i].row.Behavior = analysis.BehaviorMixed
					appPods[i].row.Recommendation = analysis.Recommendation{
						Hold:       true,
						HoldReason: "MIXED — investigate pod divergence first",
					}
				}
			}
		}

		for _, p := range appPods {
			rows = append(rows, p.row)
		}
	}

	return rows, nil
}
