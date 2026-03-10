package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set via ldflags at build time.
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of tfoutdated",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("tfoutdated %s\n", version)
		checkForUpdate()
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
