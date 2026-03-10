package fixer

import (
	"testing"
)

func TestReplaceVersionInLinePessimistic(t *testing.T) {
	line := `      version = "~> 3.75.0"`
	got := replaceVersionInLine(line, "3.75.0", "3.116.0")
	want := `      version = "~> 3.116.0"`
	if got != want {
		t.Errorf("replaceVersionInLine() = %q, want %q", got, want)
	}
}

func TestReplaceVersionInLineExact(t *testing.T) {
	line := `      version = "3.75.0"`
	got := replaceVersionInLine(line, "3.75.0", "3.116.0")
	want := `      version = "3.116.0"`
	if got != want {
		t.Errorf("replaceVersionInLine() = %q, want %q", got, want)
	}
}

func TestReplaceVersionInLineMinimum(t *testing.T) {
	line := `      version = ">= 3.75.0"`
	got := replaceVersionInLine(line, "3.75.0", "3.116.0")
	want := `      version = ">= 3.116.0"`
	if got != want {
		t.Errorf("replaceVersionInLine() = %q, want %q", got, want)
	}
}

func TestReplaceVersionInLineNoMatch(t *testing.T) {
	line := `      version = "~> 2.0.0"`
	got := replaceVersionInLine(line, "3.75.0", "3.116.0")
	// Should return unchanged
	if got != line {
		t.Errorf("replaceVersionInLine() = %q, want unchanged %q", got, line)
	}
}

func TestReplaceVersionInLineRangeConstraint(t *testing.T) {
	line := `      version = ">= 3.75.0, < 4.0.0"`
	got := replaceVersionInLine(line, "3.75.0", "3.116.0")
	want := `      version = ">= 3.116.0, < 4.0.0"`
	if got != want {
		t.Errorf("replaceVersionInLine() = %q, want %q", got, want)
	}
}

func TestReplaceVersionInLineWithPrefixes(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		oldVer  string
		newVer  string
		want    string
	}{
		{
			name:   "not-equal prefix",
			line:   `      version = "!= 3.75.0"`,
			oldVer: "3.75.0",
			newVer: "3.116.0",
			want:   `      version = "!= 3.116.0"`,
		},
		{
			name:   "less-than prefix",
			line:   `      version = "< 4.0.0"`,
			oldVer: "4.0.0",
			newVer: "5.0.0",
			want:   `      version = "< 5.0.0"`,
		},
		{
			name:   "greater-than prefix",
			line:   `      version = "> 3.0.0"`,
			oldVer: "3.0.0",
			newVer: "4.0.0",
			want:   `      version = "> 4.0.0"`,
		},
		{
			name:   "less-equal prefix",
			line:   `      version = "<= 3.75.0"`,
			oldVer: "3.75.0",
			newVer: "3.116.0",
			want:   `      version = "<= 3.116.0"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceVersionInLine(tt.line, tt.oldVer, tt.newVer)
			if got != tt.want {
				t.Errorf("replaceVersionInLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildConstraint(t *testing.T) {
	tests := []struct {
		constraintType string
		version        string
		want           string
	}{
		{"pessimistic", "3.116.0", "~> 3.116.0"},
		{"exact", "3.116.0", "3.116.0"},
		{"minimum", "3.116.0", ">= 3.116.0"},
		{"unknown", "3.116.0", "~> 3.116.0"}, // default
		{"", "3.116.0", "~> 3.116.0"},         // empty defaults to pessimistic
	}

	for _, tt := range tests {
		t.Run(tt.constraintType+"_"+tt.version, func(t *testing.T) {
			got := buildConstraint(tt.constraintType, tt.version)
			if got != tt.want {
				t.Errorf("buildConstraint(%q, %q) = %q, want %q", tt.constraintType, tt.version, got, tt.want)
			}
		})
	}
}

func TestRewriteVersions(t *testing.T) {
	content := `terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.75.0"
    }
  }
}
`
	changes := []Change{
		{
			FilePath:   "main.tf",
			Name:       "azurerm",
			OldVersion: "3.75.0",
			NewVersion: "3.116.0",
			Line:       5,
		},
	}

	result, err := rewriteVersions(content, changes)
	if err != nil {
		t.Fatalf("rewriteVersions() error: %v", err)
	}

	expected := `terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.116.0"
    }
  }
}
`
	if result != expected {
		t.Errorf("rewriteVersions() =\n%s\nwant:\n%s", result, expected)
	}
}

func TestRewriteVersionsMultipleChanges(t *testing.T) {
	content := `terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.75.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.5.0"
    }
  }
}
`
	changes := []Change{
		{Name: "azurerm", OldVersion: "3.75.0", NewVersion: "3.116.0", Line: 5},
		{Name: "random", OldVersion: "3.5.0", NewVersion: "3.6.1", Line: 9},
	}

	result, err := rewriteVersions(content, changes)
	if err != nil {
		t.Fatalf("rewriteVersions() error: %v", err)
	}

	expected := `terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.116.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.6.1"
    }
  }
}
`
	if result != expected {
		t.Errorf("rewriteVersions() =\n%s\nwant:\n%s", result, expected)
	}
}

func TestRewriteVersionsInvalidLine(t *testing.T) {
	content := "line1\nline2\n"

	changes := []Change{
		{Name: "test", OldVersion: "1.0.0", NewVersion: "2.0.0", Line: 0},  // invalid line
		{Name: "test", OldVersion: "1.0.0", NewVersion: "2.0.0", Line: 99}, // out of range
	}

	result, err := rewriteVersions(content, changes)
	if err != nil {
		t.Fatalf("rewriteVersions() error: %v", err)
	}

	// Should return unchanged content
	if result != content {
		t.Errorf("expected unchanged content for invalid lines, got %q", result)
	}
}

func TestSplitAndJoinLines(t *testing.T) {
	content := "line1\nline2\nline3"
	lines := splitLines(content)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	rejoined := joinLines(lines)
	if rejoined != content {
		t.Errorf("round-trip failed: got %q, want %q", rejoined, content)
	}
}

func TestSplitLinesTrailingNewline(t *testing.T) {
	content := "line1\nline2\n"
	lines := splitLines(content)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), lines)
	}
	rejoined := joinLines(lines)
	if rejoined != content {
		t.Errorf("round-trip failed: got %q, want %q", rejoined, content)
	}
}
