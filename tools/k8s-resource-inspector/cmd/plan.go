package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/config"
	"github.com/davidacain/platform-lab/tools/k8s-resource-inspector/pkg/plan"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Generate kri-plan.yaml with recommended resource changes",
	Long: `plan runs the inspect pipeline and writes kri-plan.yaml containing
recommended resource changes for each app. Edit the file to adjust values or
set apply: false to skip an app, then run: kri apply`,
	RunE: runPlan,
}

func init() {
	planCmd.Flags().StringVar(&window, "window", "7d", "Observation window for Prometheus queries (e.g. 7d, 24h, 1h)")
	planCmd.Flags().Float64Var(&confidenceThreshold, "confidence", 0.8, "Minimum confidence threshold for recommendations (0.0–1.0)")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dynClient, err := buildDynamicClient(kubeconfig, kubeCtx)
	if err != nil {
		return fmt.Errorf("build kubernetes client: %w", err)
	}

	apps, err := listApps(ctx, dynClient)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return nil
	}

	rows, err := buildRows(ctx, cfg, dynClient, apps, window, confidenceThreshold)
	if err != nil {
		return err
	}

	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.AppName != b.AppName {
			return a.AppName < b.AppName
		}
		return a.Container < b.Container
	})

	plans := plan.Build(rows)
	return plan.Write(plans)
}
