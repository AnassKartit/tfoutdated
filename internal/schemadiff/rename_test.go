package schemadiff

import (
	"math"
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

func TestDetectRenames_ObviousRename(t *testing.T) {
	removed := map[string]*tfconfig.Variable{
		"resource_group_name": {
			Name:        "resource_group_name",
			Type:        "string",
			Description: "The name of the resource group",
			Required:    true,
		},
	}
	added := map[string]*tfconfig.Variable{
		"resource_group_resource_id": {
			Name:        "resource_group_resource_id",
			Type:        "string",
			Description: "The resource ID of the resource group",
			Required:    true,
		},
	}

	renames := detectRenames(removed, added, &tfconfig.Module{}, &tfconfig.Module{})

	if len(renames) != 1 {
		t.Fatalf("expected 1 rename, got %d", len(renames))
	}
	if renames["resource_group_name"] != "resource_group_resource_id" {
		t.Errorf("expected resource_group_name -> resource_group_resource_id, got %v", renames)
	}
}

func TestDetectRenames_EmptyTypes(t *testing.T) {
	removed := map[string]*tfconfig.Variable{
		"vnet_name": {
			Name:        "vnet_name",
			Type:        "",
			Description: "The name of the virtual network",
			Required:    true,
		},
	}
	added := map[string]*tfconfig.Variable{
		"virtual_network_name": {
			Name:        "virtual_network_name",
			Type:        "",
			Description: "The name of the virtual network to use",
			Required:    true,
		},
	}

	renames := detectRenames(removed, added, &tfconfig.Module{}, &tfconfig.Module{})

	if len(renames) != 1 {
		t.Fatalf("expected 1 rename, got %d", len(renames))
	}
	if renames["vnet_name"] != "virtual_network_name" {
		t.Errorf("expected vnet_name -> virtual_network_name, got %v", renames)
	}
}

func TestDetectRenames_NoMatch(t *testing.T) {
	removed := map[string]*tfconfig.Variable{
		"location": {
			Name:        "location",
			Type:        "string",
			Description: "The Azure region",
			Required:    true,
		},
	}
	added := map[string]*tfconfig.Variable{
		"enable_monitoring": {
			Name:        "enable_monitoring",
			Type:        "bool",
			Description: "Whether to enable monitoring",
			Required:    false,
			Sensitive:   true,
		},
	}

	renames := detectRenames(removed, added, &tfconfig.Module{}, &tfconfig.Module{})

	if len(renames) != 0 {
		t.Errorf("expected 0 renames for unrelated variables, got %d: %v", len(renames), renames)
	}
}

func TestDetectRenames_MultipleSimultaneous(t *testing.T) {
	removed := map[string]*tfconfig.Variable{
		"resource_group_name": {
			Name:        "resource_group_name",
			Type:        "string",
			Description: "The name of the resource group",
			Required:    true,
		},
		"subnet_name": {
			Name:        "subnet_name",
			Type:        "string",
			Description: "The name of the subnet",
			Required:    false,
		},
	}
	added := map[string]*tfconfig.Variable{
		"resource_group_id": {
			Name:        "resource_group_id",
			Type:        "string",
			Description: "The ID of the resource group",
			Required:    true,
		},
		"subnet_resource_name": {
			Name:        "subnet_resource_name",
			Type:        "string",
			Description: "The name of the subnet resource",
			Required:    false,
		},
	}

	renames := detectRenames(removed, added, &tfconfig.Module{}, &tfconfig.Module{})

	if len(renames) != 2 {
		t.Fatalf("expected 2 renames, got %d: %v", len(renames), renames)
	}
	if renames["resource_group_name"] != "resource_group_id" {
		t.Errorf("expected resource_group_name -> resource_group_id, got %s", renames["resource_group_name"])
	}
	if renames["subnet_name"] != "subnet_resource_name" {
		t.Errorf("expected subnet_name -> subnet_resource_name, got %s", renames["subnet_name"])
	}
}

func TestDetectOutputRenames(t *testing.T) {
	removed := map[string]*tfconfig.Output{
		"vnet_id": {
			Name:        "vnet_id",
			Description: "The ID of the virtual network",
		},
	}
	added := map[string]*tfconfig.Output{
		"virtual_network_id": {
			Name:        "virtual_network_id",
			Description: "The ID of the virtual network",
		},
	}

	renames := detectOutputRenames(removed, added)

	if len(renames) != 1 {
		t.Fatalf("expected 1 output rename, got %d", len(renames))
	}
	if renames["vnet_id"] != "virtual_network_id" {
		t.Errorf("expected vnet_id -> virtual_network_id, got %v", renames)
	}
}

func TestNameSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"resource_group_name", "resource_group_name", 1.0, 1.0},
		{"resource_group_name", "resource_group_id", 0.4, 0.9},
		{"location", "enable_monitoring", 0.0, 0.2},
		{"vnet_name", "virtual_network_name", 0.2, 0.8},
	}

	for _, tt := range tests {
		score := nameSimilarity(tt.a, tt.b)
		if score < tt.wantMin || score > tt.wantMax {
			t.Errorf("nameSimilarity(%q, %q) = %f, want [%f, %f]",
				tt.a, tt.b, score, tt.wantMin, tt.wantMax)
		}
	}
}

func TestTypeCompatibility(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"string", "string", 1.0},
		{"string", "", 0.5},
		{"", "", 0.5},
		{"list(string)", "set(string)", 0.6},
		{"string", "bool", 0.6}, // same "scalar" family
		{"map(string)", "object({})", 0.6},
	}

	for _, tt := range tests {
		got := typeCompatibility(tt.a, tt.b)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("typeCompatibility(%q, %q) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDescriptionSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"The name of the resource group", "The name of the resource group", 1.0, 1.0},
		{"The name of the resource group", "", 0.5, 0.5},
		{"", "", 0.5, 0.5},
		{"The name of the resource group", "The ID of the resource group", 0.3, 0.8},
		{"completely different text", "nothing alike here", 0.0, 0.1},
	}

	for _, tt := range tests {
		got := descriptionSimilarity(tt.a, tt.b)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("descriptionSimilarity(%q, %q) = %f, want [%f, %f]",
				tt.a, tt.b, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestDefaultSimilarity(t *testing.T) {
	tests := []struct {
		a, b interface{}
		want float64
	}{
		{nil, nil, 1.0},
		{nil, "hello", 0.3},
		{"hello", "hello", 1.0},
		{"hello", "world", 0.5}, // same Go type (string)
		{"hello", 42, 0.0},     // different Go type
	}

	for _, tt := range tests {
		got := defaultSimilarity(tt.a, tt.b)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("defaultSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRequiredMatch(t *testing.T) {
	if got := requiredMatch(true, true); got != 1.0 {
		t.Errorf("requiredMatch(true, true) = %f, want 1.0", got)
	}
	if got := requiredMatch(true, false); got != 0.2 {
		t.Errorf("requiredMatch(true, false) = %f, want 0.2", got)
	}
}

func TestSensitiveMatch(t *testing.T) {
	if got := sensitiveMatch(true, true); got != 1.0 {
		t.Errorf("sensitiveMatch(true, true) = %f, want 1.0", got)
	}
	if got := sensitiveMatch(true, false); got != 0.0 {
		t.Errorf("sensitiveMatch(true, false) = %f, want 0.0", got)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"sunday", "saturday", 3},
	}

	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestLCS(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 3},
		{"abc", "def", 0},
		{"abcde", "ace", 3},
		{"resource_group_name", "resource_group_id", 15},
	}

	for _, tt := range tests {
		got := lcs(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("lcs(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestComputeVariableScore_AboveThreshold(t *testing.T) {
	oldVar := &tfconfig.Variable{
		Type:        "string",
		Description: "The name of the resource group",
		Required:    true,
	}
	newVar := &tfconfig.Variable{
		Type:        "string",
		Description: "The resource ID of the resource group",
		Required:    true,
	}

	score := computeVariableScore("resource_group_name", "resource_group_resource_id", oldVar, newVar)
	if score < 0.45 {
		t.Errorf("expected score >= 0.45 for obvious rename, got %f", score)
	}
}

func TestComputeVariableScore_BelowThreshold(t *testing.T) {
	oldVar := &tfconfig.Variable{
		Type:        "string",
		Description: "The Azure region",
		Required:    true,
	}
	newVar := &tfconfig.Variable{
		Type:        "bool",
		Description: "Whether to enable monitoring",
		Required:    false,
		Sensitive:   true,
	}

	score := computeVariableScore("location", "enable_monitoring", oldVar, newVar)
	if score >= 0.45 {
		t.Errorf("expected score < 0.45 for unrelated variables, got %f", score)
	}
}
