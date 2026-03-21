package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/analysis"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/argo"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/config"
	gitvals "github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/git"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/hpa"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/metrics"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/output"
	"github.com/dcain/platform-lab/tools/k8s-resource-inspector/pkg/pods"
	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var window string
var confidenceThreshold float64

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect workload resource utilization",
	RunE:  runInspect,
}

func init() {
	inspectCmd.Flags().StringVar(&window, "window", "7d", "Observation window for Prometheus queries (e.g. 7d, 24h, 1h)")
	inspectCmd.Flags().Float64Var(&confidenceThreshold, "confidence", 0.8, "Minimum confidence threshold for recommendations (0.0–1.0)")
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dynClient, err := buildDynamicClient(kubeconfig, kubeCtx)
	if err != nil {
		return fmt.Errorf("build kubernetes client: %w", err)
	}

	apps, err := argo.List(ctx, dynClient, "argocd")
	if err != nil {
		return fmt.Errorf("list ArgoCD applications: %w", err)
	}

	if len(apps) == 0 {
		fmt.Println("No ArgoCD applications found.")
		return nil
	}

	if appFilter != "" {
		filtered := apps[:0]
		for _, a := range apps {
			if a.Name == appFilter {
				filtered = append(filtered, a)
			}
		}
		apps = filtered
	}

	if namespace != "" {
		filtered := apps[:0]
		for _, a := range apps {
			if a.Namespace == namespace {
				filtered = append(filtered, a)
			}
		}
		apps = filtered
	}

	if len(apps) == 0 {
		fmt.Println("No applications match the specified filters.")
		return nil
	}

	var rows []output.PodRow

	for _, app := range apps {
		promURL, ok := cfg.PrometheusFor(app.DestinationName)
		if !ok {
			fmt.Fprintf(os.Stderr, "warn: no Prometheus configured for cluster %q (app %s), skipping\n",
				app.DestinationName, app.Name)
			continue
		}

		lister, err := pods.NewPromLister(promURL)
		if err != nil {
			return fmt.Errorf("create pod lister for app %s: %w", app.Name, err)
		}

		podList, err := lister.ListPods(ctx, app.Namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: list pods for app %s: %v\n", app.Name, err)
			continue
		}

		metricsClient, err := metrics.NewClient(promURL)
		if err != nil {
			return fmt.Errorf("create metrics client for app %s: %w", app.Name, err)
		}

		usageMap, err := metricsClient.UsageMetrics(ctx, app.Namespace, window)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: fetch usage metrics for app %s: %v\n", app.Name, err)
		}

		// Read Helm values from git to get the configured resource values and file path.
		var gitConfig *gitvals.ValuesConfig
		if app.RepoURL != "" && len(app.ValueFiles) > 0 {
			gitConfig, err = gitvals.ReadValues(app.RepoURL, app.TargetRevision, app.Path, app.ValueFiles[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: read git values for app %s: %v\n", app.Name, err)
			}
		}

		// Fetch HPAs for this namespace — used to join to pods by scaleTargetRef.
		hpas, err := hpa.List(ctx, dynClient, app.Namespace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: list HPAs for app %s: %v\n", app.Name, err)
		}
		appHPA := hpa.FindForTarget(hpas, app.Name)

		// Classify each pod and collect behaviors for workload-level aggregation.
		type podWithBehavior struct {
			row      output.PodRow
			behavior analysis.BehaviorClass
		}
		var appPods []podWithBehavior

		for _, p := range podList {
			row := output.PodRow{
				AppName:   app.Name,
				Cluster:   app.DestinationName,
				Namespace: p.Namespace,
				PodName:   p.PodName,
				Container: p.ContainerName,
				CPUReq:    p.CPURequest,
				CPULim:    p.CPULimit,
				MemReq:    p.MemRequest,
				MemLim:    p.MemLimit,
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
				row.Recommendation = analysis.Recommend(behavior, confidence, confidenceThreshold, u, p.CPURequest, p.MemRequest, p.MemLimit)

				// Annotate recommendation with the values file path when available.
				if gitConfig != nil && !row.Recommendation.Hold && row.Recommendation.Text != "" {
					row.ValuesFilePath = gitConfig.FilePath
				}
			} else {
				row.Behavior = analysis.BehaviorUnknown
				row.HPAStatus = hpa.Validate(appHPA, metrics.Usage{}, p.CPURequest, p.MemRequest, analysis.BehaviorUnknown)
			}

			appPods = append(appPods, podWithBehavior{row: row, behavior: row.Behavior})
		}

		// Workload-level MIXED detection: if pods in this app disagree, override
		// both behavior and recommendation for all pods in the workload.
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

	return output.PrintTable(rows)
}

func buildDynamicClient(kubeconfigPath, kubeCtx string) (dynamic.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeCtx != "" {
		configOverrides.CurrentContext = kubeCtx
	}

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(restConfig)
}
