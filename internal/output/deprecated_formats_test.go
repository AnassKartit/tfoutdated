package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// Every output format must tell the reader a module is deprecated, what
// replaces it and why — including when it is already on its latest version,
// the case most likely to be read as "all good".
func TestEveryFormatReportsDeprecation(t *testing.T) {
	const (
		source     = "Azure/avm-ptn-virtualwan/azurerm"
		replacedBy = "Azure/avm-ptn-alz-connectivity-virtual-wan/azurerm//modules/virtual-wan"
		reason     = "AVM index: Deprecated; GitHub repository archived (last push 2026-09-18)"
	)
	a := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{
				Name: "vwan", Source: source, FilePath: "main.tf", Line: 7, IsModule: true,
				CurrentVer: "0.14.1", LatestVer: "0.14.1", UpdateType: resolver.UpdateNone,
				Deprecated: true, ReplacedBy: replacedBy, DeprecationReason: reason,
			},
			{
				Name: "azurerm", Source: "hashicorp/azurerm", FilePath: "versions.tf", Line: 3,
				CurrentVer: "3.116.0", LatestVer: "4.40.0", UpdateType: resolver.UpdateMajor,
			},
		},
		TotalDeps: 2,
	}

	for _, format := range []string{"table", "json", "markdown", "html", "github", "azdevops"} {
		t.Run(format, func(t *testing.T) {
			r, err := New(format, Options{NoColor: true})
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := r.Render(&buf, a); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := buf.String()
			for _, want := range []string{source, replacedBy, "archived"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s output is missing %q", format, want)
				}
			}
		})
	}
}

// The plan bar draws each dependency once: replace wins over an available
// upgrade, so the segments add up to the total.
func TestHTMLPlanCountsEachDependencyOnce(t *testing.T) {
	a := &analyzer.Analysis{
		TotalDeps: 3,
		Dependencies: []analyzer.DependencyAnalysis{
			{Name: "old", Source: "Azure/avm-ptn-virtualwan/azurerm", IsModule: true, Deprecated: true,
				CurrentVer: "0.14.0", LatestVer: "0.14.1", UpdateType: resolver.UpdatePatch},
			{Name: "vpc", Source: "terraform-aws-modules/vpc/aws", IsModule: true,
				CurrentVer: "5.0.0", LatestVer: "6.7.3", UpdateType: resolver.UpdateMajor},
		},
	}
	var buf bytes.Buffer
	if err := (&HTMLRenderer{}).Render(&buf, a); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"1 to replace", "1 major", "0 patch", "1 current", "of 3 dependencies"} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML plan is missing %q", want)
		}
	}
	_, upgrade, found := strings.Cut(out, `id="upgrade"`)
	if !found {
		t.Fatal("upgrade section missing for vpc")
	}
	if strings.Contains(upgrade, "avm-ptn-virtualwan") {
		t.Error("deprecated module should be listed under Replace only, not under Upgrade")
	}
}
