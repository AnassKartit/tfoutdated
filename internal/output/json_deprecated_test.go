package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// A deprecated module already on its latest version must stay in the JSON:
// pipelines and agents read this output, not the table.
func TestJSONKeepsDeprecatedUpToDate(t *testing.T) {
	a := &analyzer.Analysis{Dependencies: []analyzer.DependencyAnalysis{
		{Name: "vwan", Source: "Azure/avm-ptn-virtualwan/azurerm", UpdateType: resolver.UpdateNone, Deprecated: true,
			ReplacedBy: "Azure/avm-ptn-alz-connectivity-virtual-wan/azurerm//modules/virtual-wan", DeprecationReason: "AVM index: Deprecated"},
		{Name: "current", Source: "hashicorp/random", UpdateType: resolver.UpdateNone},
	}}

	var buf bytes.Buffer
	if err := (&JSONRenderer{}).Render(&buf, a); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Dependencies []struct {
			Name              string `json:"name"`
			Deprecated        bool   `json:"deprecated"`
			ReplacedBy        string `json:"replaced_by"`
			DeprecationReason string `json:"deprecation_reason"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Dependencies) != 1 || out.Dependencies[0].Name != "vwan" {
		t.Fatalf("dependencies = %+v, want only the deprecated module", out.Dependencies)
	}
	d := out.Dependencies[0]
	if !d.Deprecated || d.ReplacedBy == "" || d.DeprecationReason == "" {
		t.Errorf("deprecated fields missing: %+v", d)
	}
}
