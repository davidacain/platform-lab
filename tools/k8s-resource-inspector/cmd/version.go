package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.1.0-phase1"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print kri version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("kri", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
