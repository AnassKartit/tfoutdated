package cmd

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

func TestNormalizeArgs(t *testing.T) {
	long := map[string]bool{"fail-on": true, "auto-approve": true, "out": true}
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"terraform style with value", []string{"check", "-fail-on=deprecated"}, []string{"check", "--fail-on=deprecated"}},
		{"terraform style boolean", []string{"apply", "-auto-approve"}, []string{"apply", "--auto-approve"}},
		{"separate value", []string{"check", "-out", "r.html"}, []string{"check", "--out", "r.html"}},
		{"double dash untouched", []string{"check", "--fail-on=major"}, []string{"check", "--fail-on=major"}},
		{"shorthand untouched", []string{"check", "-v", "-o", "json"}, []string{"check", "-v", "-o", "json"}},
		{"unknown single dash untouched", []string{"check", "-rv"}, []string{"check", "-rv"}},
		{"after -- untouched", []string{"check", "--", "-fail-on"}, []string{"check", "--", "-fail-on"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeArgs(tt.in, long); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("normalizeArgs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestApplyEnv(t *testing.T) {
	var failOn []string
	var format string
	c := &cobra.Command{Use: "x", Run: func(*cobra.Command, []string) {}}
	c.Flags().StringSliceVar(&failOn, "fail-on", nil, "")
	c.Flags().StringVar(&format, "format", "table", "")

	t.Setenv("TFOUTDATED_FAIL_ON", "deprecated,major")
	t.Setenv("TFOUTDATED_FORMAT", "json")
	if err := c.ParseFlags([]string{"--format=markdown"}); err != nil {
		t.Fatal(err)
	}
	if err := applyEnv(c); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(failOn, []string{"deprecated", "major"}) {
		t.Errorf("fail-on from env = %q", failOn)
	}
	if format != "markdown" {
		t.Errorf("format = %q, want the command-line value to win over the environment", format)
	}
}

func TestGateHits(t *testing.T) {
	a := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{Name: "vwan", Deprecated: true, UpdateType: resolver.UpdateNone},
			{Name: "azurerm", UpdateType: resolver.UpdateMajor},
			{Name: "random", UpdateType: resolver.UpdatePatch},
		},
		BreakingChanges: []breaking.BreakingChange{
			{Provider: "azurerm", Severity: breaking.SeverityBreaking},
			{Provider: "azurerm", Severity: breaking.SeverityInfo}, // not a gate hit
		},
	}

	got := gateHits(a, []string{"deprecated", "major", "breaking", "minor", "any"})
	want := map[string]int{"deprecated": 1, "major": 1, "breaking": 1, "any": 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gateHits = %v, want %v", got, want)
	}
}

func TestGateResultAcrossRepos(t *testing.T) {
	g := gate{enabled: true, cats: []string{"deprecated"}}
	g.record(&analyzer.Analysis{Dependencies: []analyzer.DependencyAnalysis{{Deprecated: true}}})
	g.record(&analyzer.Analysis{Dependencies: []analyzer.DependencyAnalysis{{Deprecated: true}}})

	err := g.result()
	if ExitCode(err) != ExitFindings {
		t.Fatalf("exit code = %d, want %d", ExitCode(err), ExitFindings)
	}
	if err.Error() != "fail-on: 2 deprecated" {
		t.Errorf("message = %q", err.Error())
	}

	clean := gate{enabled: true, cats: []string{"deprecated"}}
	clean.record(&analyzer.Analysis{Dependencies: []analyzer.DependencyAnalysis{{UpdateType: resolver.UpdatePatch}}})
	if err := clean.result(); err != nil {
		t.Errorf("no deprecated module, want nil, got %v", err)
	}
}

func TestValidateGate(t *testing.T) {
	if err := validateGate([]string{"deprecated", "MAJOR"}); err != nil {
		t.Errorf("valid categories rejected: %v", err)
	}
	if err := validateGate([]string{"outdatd"}); err == nil {
		t.Error("typo accepted")
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(nil) != ExitOK {
		t.Error("nil should be ExitOK")
	}
	if ExitCode(errors.New("boom")) != ExitError {
		t.Error("plain error should be ExitError")
	}
	wrapped := fmt.Errorf("wrap: %w", &ExitCodeError{Code: ExitFindings})
	if ExitCode(wrapped) != ExitFindings {
		t.Error("wrapped ExitCodeError should keep its code")
	}
}
