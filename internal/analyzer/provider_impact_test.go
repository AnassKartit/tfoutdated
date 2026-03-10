package analyzer

import (
	"testing"
)

func TestSatisfiesConstraint(t *testing.T) {
	tests := []struct {
		version    string
		constraint string
		expected   bool
	}{
		{"4.0.0", "~> 4.0", true},
		{"4.5.0", "~> 4.0", true},
		{"5.0.0", "~> 4.0", false},
		{"3.99.0", "~> 4.0", false},
		{"4.0.1", ">= 4.0.0", true},
		{"3.99.0", ">= 4.0.0", false},
		{"4.0.0", ">= 3.0, < 5.0", true},
		{"5.0.0", ">= 3.0, < 5.0", false},
		{"4.0.0", "= 4.0.0", true},
		{"4.0.1", "= 4.0.0", false},
	}

	for _, tt := range tests {
		got := satisfiesConstraint(tt.version, tt.constraint)
		if got != tt.expected {
			t.Errorf("satisfiesConstraint(%q, %q) = %v, want %v",
				tt.version, tt.constraint, got, tt.expected)
		}
	}
}

func TestSatisfiesPessimistic(t *testing.T) {
	tests := []struct {
		version  string
		base     string
		expected bool
	}{
		// ~> 4.0 means >= 4.0, < 5.0
		{"4.0.0", "4.0", true},
		{"4.5.0", "4.0", true},
		{"5.0.0", "4.0", false},
		{"3.99.0", "4.0", false},

		// ~> 4.0.1 means >= 4.0.1, < 4.1.0
		{"4.0.1", "4.0.1", true},
		{"4.0.5", "4.0.1", true},
		{"4.1.0", "4.0.1", false},
		{"4.0.0", "4.0.1", false},
	}

	for _, tt := range tests {
		got := satisfiesPessimistic(tt.version, tt.base)
		if got != tt.expected {
			t.Errorf("satisfiesPessimistic(%q, %q) = %v, want %v",
				tt.version, tt.base, got, tt.expected)
		}
	}
}

func TestCompareVersionNums(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"4.0.0", "3.0.0", 1},
		{"3.0.0", "4.0.0", -1},
		{"4.0.0", "4.0.0", 0},
		{"4.1.0", "4.0.0", 1},
		{"4.0.1", "4.0.0", 1},
	}

	for _, tt := range tests {
		got := compareVersionNums(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("compareVersionNums(%q, %q) = %d, want %d",
				tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestIsCompatibleNoProviderDep(t *testing.T) {
	// Module doesn't depend on the provider — should be compatible
	got := isCompatible(nil, "hashicorp/azurerm", "4.0.0")
	if !got {
		t.Error("expected compatible when no provider deps")
	}
}

func TestPiParseVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected []int
	}{
		{"3.75.0", []int{3, 75, 0}},
		{"v4.0.0", []int{4, 0, 0}},
		{"1.2.3-beta1", []int{1, 2, 3}},
		{"", nil},
		{"abc", nil},
	}

	for _, tt := range tests {
		got := piParseVersion(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("piParseVersion(%q) = %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("piParseVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}
