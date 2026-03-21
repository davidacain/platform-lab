package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

var (
	kubeconfig string
	kubeCtx    string
	namespace  string
	outputFmt  string
	configFile string
)

var rootCmd = &cobra.Command{
	Use:     "hvc",
	Short:   "Check Helm release versions against upstream chart repositories",
	Version: Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (default: ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&kubeCtx, "context", "", "Kubeconfig context to use (default: current context)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Limit to a single namespace (default: all)")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format: table or json")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file (default: ~/.hvc/config.yaml)")

	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(checkCmd)
}
