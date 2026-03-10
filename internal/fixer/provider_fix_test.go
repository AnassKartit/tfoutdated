package fixer

import "testing"

func TestExtractMinVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"~> 3.75.0", "3.75.0"},
		{">= 4.0", "4.0"},
		{">= 3.0, < 5.0", "3.0"},
		{"3.75.0", "3.75.0"},
		{"> 2.0", "2.0"},
		{"~> 3.0", "3.0"},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractMinVersion(tt.input)
		if got != tt.want {
			t.Errorf("extractMinVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		input                  string
		wantMaj, wantMin, wantPat int
	}{
		{"3.75.0", 3, 75, 0},
		{"4.0", 4, 0, 0},
		{"1", 1, 0, 0},
		{"3.116.2", 3, 116, 2},
	}
	for _, tt := range tests {
		maj, min, pat := parseVersionParts(tt.input)
		if maj != tt.wantMaj || min != tt.wantMin || pat != tt.wantPat {
			t.Errorf("parseVersionParts(%q) = (%d,%d,%d), want (%d,%d,%d)",
				tt.input, maj, min, pat, tt.wantMaj, tt.wantMin, tt.wantPat)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int // sign only
	}{
		{"3.75.0", "3.75.0", 0},
		{"3.76.0", "3.75.0", 1},
		{"3.75.0", "3.76.0", -1},
		{"4.0.0", "3.99.0", 1},
		{"3.0.0", "4.0.0", -1},
	}
	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if (tt.want == 0 && got != 0) || (tt.want > 0 && got <= 0) || (tt.want < 0 && got >= 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsConstraintCompatible(t *testing.T) {
	tests := []struct {
		constraint  string
		requiredMin string
		want        bool
	}{
		// Pessimistic ~> X.Y.Z: allows [X.Y.Z, X.(Y+1).0)
		{"~> 3.75.0", "3.75.0", true},
		{"~> 3.75.0", "3.75.5", true},
		{"~> 3.75.0", "3.76.0", false},  // new minor needed
		{"~> 3.75.0", "3.80.0", false},
		{"~> 3.75.0", "4.0.0", false},

		// Pessimistic ~> X.Y: allows [X.Y.0, (X+1).0.0)
		{"~> 3.0", "3.0.0", true},
		{"~> 3.0", "3.80.0", true},
		{"~> 3.0", "4.0.0", false},

		// Minimum >= X.Y.Z
		{">= 3.75.0", "3.75.0", true},
		{">= 3.75.0", "3.80.0", true},
		{">= 3.75.0", "4.0.0", true},
		{">= 3.75.0", "3.74.0", false},

		// Range >= X, < Y
		{">= 3.0, < 5.0", "3.80.0", true},
		{">= 3.0, < 5.0", "5.0.0", false},
		{">= 3.0, < 5.0", "4.99.0", true},

		// Bare version
		{"3.75.0", "3.75.0", true},
		{"3.75.0", "3.76.0", false},
	}
	for _, tt := range tests {
		got := isConstraintCompatible(tt.constraint, tt.requiredMin)
		if got != tt.want {
			t.Errorf("isConstraintCompatible(%q, %q) = %v, want %v",
				tt.constraint, tt.requiredMin, got, tt.want)
		}
	}
}

func TestBuildNewConstraint(t *testing.T) {
	tests := []struct {
		oldConstraint string
		requiredMin   string
		want          string
	}{
		// Pessimistic three-part
		{"~> 3.75.0", "3.80.0", "~> 3.80.0"},
		{"~> 3.75.0", "3.116.0", "~> 3.116.0"},

		// Pessimistic two-part
		{"~> 3.0", "3.80", "~> 3.80"},

		// Minimum
		{">= 3.75.0", "3.80.0", ">= 3.80.0"},

		// Range with valid upper bound
		{">= 3.0, < 5.0", "3.80.0", ">= 3.80.0, < 5.0"},

		// Range where upper bound is no longer valid
		{">= 3.0, < 3.5", "3.80.0", ">= 3.80.0"},

		// Bare version -> pessimistic
		{"3.75.0", "3.80.0", "~> 3.80.0"},
	}
	for _, tt := range tests {
		got := buildNewConstraint(tt.oldConstraint, tt.requiredMin)
		if got != tt.want {
			t.Errorf("buildNewConstraint(%q, %q) = %q, want %q",
				tt.oldConstraint, tt.requiredMin, got, tt.want)
		}
	}
}

func TestUnique(t *testing.T) {
	got := unique([]string{"a", "b", "a", "c", "b"})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("unique() = %v, want [a b c]", got)
	}
}
