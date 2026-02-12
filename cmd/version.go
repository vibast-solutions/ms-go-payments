package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and build information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("payments-service %s (commit: %s)\n", Version, Commit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
