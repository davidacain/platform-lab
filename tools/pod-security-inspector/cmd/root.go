package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	namespace    string
	outputFmt    string
	kubeconfig   string
	kubeCtx      string
	findingsOnly bool
)

var rootCmd = &cobra.Command{
	Use:     "pod-security-inspector",
	Short:   "Inspect pod security contexts and Istio mesh status across namespaces",
	Version: Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Limit to a single namespace (default: all)")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format: table or json")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&kubeCtx, "context", "", "Kubeconfig context to use (default: current context)")
	rootCmd.PersistentFlags().BoolVar(&findingsOnly, "findings-only", false, "Suppress rows/pods with no issues")

	rootCmd.AddCommand(securityCmd)
	rootCmd.AddCommand(meshCmd)
	rootCmd.AddCommand(netpolCmd)
	rootCmd.AddCommand(allCmd)
}
