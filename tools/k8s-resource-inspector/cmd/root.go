package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfig string
	kubeCtx    string
	configPath string
	outputFmt  string
	namespace  string
	appFilter  string
)

var rootCmd = &cobra.Command{
	Use:   "kri",
	Short: "Kubernetes resource inspector — analyze workload resource utilization",
	Long: `kri analyzes Kubernetes workload resource utilization by combining live pod
metrics from Prometheus with resource configuration read from ArgoCD Application CRs.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&kubeCtx, "context", "", "Kubeconfig context (default: current context)")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Config file path (default: ~/.kri/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format: table, json")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Scope to a single namespace")
	rootCmd.PersistentFlags().StringVar(&appFilter, "app", "", "Scope to a single ArgoCD application")
}
