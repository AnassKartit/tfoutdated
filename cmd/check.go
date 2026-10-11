package cmd

import (
	"github.com/spf13/cobra"
)

var flagFailOn []string

var checkCmd = &cobra.Command{
	Use:   "check [dir]",
	Short: "Report outdated, deprecated and archived modules and providers",
	Long: `Check reads the Terraform in a directory and reports what is outdated,
which modules are deprecated, archived or moved upstream, and which upgrades
carry breaking changes. It changes nothing.

Exit codes follow terraform: 0 nothing to report, 1 error, 2 a finding matched
-fail-on. Without -fail-on, check only reports and exits 0.`,
	Example: `  tfoutdated check
  tfoutdated check ./infra -fail-on=deprecated,major
  tfoutdated check -repos=repos.txt -out=report.html
  TFOUTDATED_FAIL_ON=deprecated tfoutdated check`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateGate(flagFailOn); err != nil {
			return err
		}
		activeGate = gate{enabled: true, cats: flagFailOn}
		if err := runScan(cmd, args); err != nil {
			return err
		}
		return activeGate.result()
	},
}

func init() {
	checkCmd.Flags().StringSliceVar(&flagFailOn, "fail-on", nil, "Exit 2 when findings match: "+gateHelp())
	rootCmd.AddCommand(checkCmd)
}
