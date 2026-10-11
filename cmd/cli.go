package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// Exit codes for check, plan and apply, following terraform's
// -detailed-exitcode: 0 nothing to report, 1 error, 2 findings.
const (
	ExitOK       = 0
	ExitError    = 1
	ExitFindings = 2
)

// ExitCodeError carries a process exit code through cobra's error path, so
// commands return instead of calling os.Exit and deferred cleanup still runs.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string { return e.Message }

// ExitCode maps an error returned by Execute to the process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	return ExitError
}

// envPrefix namespaces the environment variables that set flags, so a pipeline
// can configure the tool without arguments: TFOUTDATED_FAIL_ON=deprecated.
const envPrefix = "TFOUTDATED_"

// envName returns the variable that sets a flag: "fail-on" -> TFOUTDATED_FAIL_ON.
func envName(flag string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
}

// applyEnv sets every flag the user did not pass from its environment variable.
// Command-line arguments always win.
func applyEnv(cmd *cobra.Command) error {
	var errs []string
	apply := func(f *pflag.Flag) {
		if f.Changed {
			return
		}
		v, ok := os.LookupEnv(envName(f.Name))
		if !ok {
			return
		}
		if err := f.Value.Set(v); err != nil {
			errs = append(errs, fmt.Sprintf("%s=%q: %v", envName(f.Name), v, err))
			return
		}
		f.Changed = true
	}
	cmd.Flags().VisitAll(apply)
	cmd.InheritedFlags().VisitAll(apply)
	if len(errs) > 0 {
		return fmt.Errorf("invalid environment: %s", strings.Join(errs, "; "))
	}
	return nil
}

// normalizeArgs accepts terraform's single-dash long flags ("-fail-on=x",
// "-auto-approve") by rewriting them to the double-dash form cobra parses.
// Single letters and shorthand clusters ("-v", "-rv") are left alone, as is
// everything after "--".
func normalizeArgs(args []string, longFlags map[string]bool) []string {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			return append(out, args[i:]...)
		}
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			name, _, _ := strings.Cut(a[1:], "=")
			if longFlags[name] {
				a = "-" + a
			}
		}
		out = append(out, a)
	}
	return out
}

// longFlagNames collects every long flag name defined on the command tree.
func longFlagNames(root *cobra.Command) map[string]bool {
	names := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		add := func(f *pflag.Flag) { names[f.Name] = true }
		c.Flags().VisitAll(add)
		c.PersistentFlags().VisitAll(add)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return names
}

// Gate categories accepted by -fail-on.
var gateCategories = map[string]string{
	"deprecated": "module deprecated, archived or announced end of life",
	"breaking":   "upgrade has breaking changes",
	"major":      "major version behind",
	"minor":      "minor version behind",
	"patch":      "patch version behind",
	"any":        "any finding",
}

func gateHelp() string {
	keys := make([]string, 0, len(gateCategories))
	for k := range gateCategories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func validateGate(cats []string) error {
	for _, c := range cats {
		if _, ok := gateCategories[strings.ToLower(strings.TrimSpace(c))]; !ok {
			return fmt.Errorf("unknown -fail-on value %q (use: %s)", c, gateHelp())
		}
	}
	return nil
}

// gateHits returns, per requested category, how many findings match.
func gateHits(a *analyzer.Analysis, cats []string) map[string]int {
	hits := map[string]int{}
	if a == nil {
		return hits
	}
	want := map[string]bool{}
	for _, c := range cats {
		want[strings.ToLower(strings.TrimSpace(c))] = true
	}
	for _, dep := range a.Dependencies {
		if want["deprecated"] && dep.Deprecated {
			hits["deprecated"]++
		}
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			if want["major"] {
				hits["major"]++
			}
		case resolver.UpdateMinor:
			if want["minor"] {
				hits["minor"]++
			}
		case resolver.UpdatePatch:
			if want["patch"] {
				hits["patch"]++
			}
		}
		if want["any"] && (dep.Deprecated || dep.UpdateType > resolver.UpdateNone) {
			hits["any"]++
		}
	}
	if want["breaking"] {
		// Same threshold as HasBreakingChanges: informational notes do not count.
		for _, bc := range a.BreakingChanges {
			if bc.Severity >= breaking.SeverityBreaking {
				hits["breaking"]++
			}
		}
	}
	return hits
}

// gate accumulates findings across every analysis a command renders (one per
// repository or path), so -fail-on works the same for single and multi-repo runs.
type gate struct {
	enabled bool
	cats    []string
	hits    map[string]int
}

var activeGate gate

func (g *gate) record(a *analyzer.Analysis) {
	if !g.enabled || len(g.cats) == 0 {
		return
	}
	if g.hits == nil {
		g.hits = map[string]int{}
	}
	for k, v := range gateHits(a, g.cats) {
		g.hits[k] += v
	}
}

// result turns accumulated hits into the command's exit status.
func (g *gate) result() error {
	if len(g.hits) == 0 {
		return nil
	}
	keys := make([]string, 0, len(g.hits))
	for k := range g.hits {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", g.hits[k], k))
	}
	return &ExitCodeError{
		Code:    ExitFindings,
		Message: "fail-on: " + strings.Join(parts, ", "),
	}
}
