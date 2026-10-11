package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var flagAutoApprove bool

var planCmd = &cobra.Command{
	Use:   "plan [dir]",
	Short: "Show the upgrades and code fixes apply would make, without changing files",
	Long: `Plan resolves upgrades and the code transforms that go with them (renamed
variables, provider constraints) and prints them. No file is modified.`,
	Example: `  tfoutdated plan
  tfoutdated plan ./infra -safe`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flagDryRun = true
		return runFix(cmd, args)
	},
}

var applyCmd = &cobra.Command{
	Use:   "apply [dir]",
	Short: "Upgrade dependencies and fix the code that uses them",
	Long: `Apply shows the plan, asks for confirmation, then rewrites version
constraints and applies the code transforms. In a pipeline, or anywhere without
a terminal, pass -auto-approve.`,
	Example: `  tfoutdated apply
  tfoutdated apply ./infra -safe
  tfoutdated apply -auto-approve`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !flagAutoApprove {
			if !isInteractive() {
				return fmt.Errorf("apply needs confirmation: run it in a terminal or pass -auto-approve")
			}
			flagDryRun = true
			if err := runFix(cmd, args); err != nil {
				return err
			}
			if !confirm("Do you want to apply these changes?\n  Only 'yes' will be accepted to approve.\n\n  Enter a value: ") {
				// Like terraform: a cancelled apply is not a success.
				return &ExitCodeError{Code: ExitError, Message: "apply cancelled"}
			}
		}
		flagDryRun = false
		return runFix(cmd, args)
	},
}

func init() {
	for _, c := range []*cobra.Command{planCmd, applyCmd} {
		c.Flags().BoolVar(&flagFixSafe, "safe", false, "Only upgrade to versions without breaking changes")
		rootCmd.AddCommand(c)
	}
	applyCmd.Flags().BoolVar(&flagAutoApprove, "auto-approve", false, "Skip the confirmation prompt")
}

// isInteractive reports whether a person can answer a prompt. A character
// device is not enough: /dev/null is one, and CI agents often attach it.
func isInteractive() bool {
	for _, v := range []string{"CI", "TF_BUILD", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "JENKINS_URL"} {
		if os.Getenv(v) != "" {
			return false
		}
	}
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func confirm(prompt string) bool {
	fmt.Print("\n" + prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line) == "yes"
}
