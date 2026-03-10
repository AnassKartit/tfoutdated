package resolver

import (
	"testing"

	goversion "github.com/hashicorp/go-version"
)

func mustVersion(t *testing.T, s string) *goversion.Version {
	t.Helper()
	v, err := goversion.NewVersion(s)
	if err != nil {
		t.Fatalf("invalid version %q: %v", s, err)
	}
	return v
}

func TestParseCurrentVersion(t *testing.T) {
	tests := []struct {
		constraint string
		want       string
	}{
		{"3.75.0", "3.75.0"},
		{"~> 3.75.0", "3.75.0"},
		{">= 3.75.0", "3.75.0"},
		{"<= 3.75.0", "3.75.0"},
		{"!= 3.75.0", "3.75.0"},
		{"> 3.75.0", "3.75.0"},
		{"< 3.75.0", "3.75.0"},
		{"= 3.75.0", "3.75.0"},
		{">= 3.75.0, < 4.0.0", "3.75.0"},
		{"~> 0.6.0", "0.6.0"},
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			got := parseCurrentVersion(tt.constraint)
			if got == nil {
				t.Fatalf("parseCurrentVersion(%q) returned nil", tt.constraint)
			}
			if got.String() != tt.want {
				t.Errorf("parseCurrentVersion(%q) = %q, want %q", tt.constraint, got.String(), tt.want)
			}
		})
	}
}

func TestParseCurrentVersionInvalid(t *testing.T) {
	got := parseCurrentVersion("not-a-version")
	if got != nil {
		t.Errorf("parseCurrentVersion(invalid) = %v, want nil", got)
	}
}

func TestClassifyUpdate(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    UpdateType
	}{
		{"same version", "3.75.0", "3.75.0", UpdateNone},
		{"patch update", "3.75.0", "3.75.1", UpdatePatch},
		{"minor update", "3.75.0", "3.76.0", UpdateMinor},
		{"major update", "3.75.0", "4.0.0", UpdateMajor},
		{"latest is older", "4.0.0", "3.75.0", UpdateNone},
		{"minor with patch", "3.75.0", "3.76.1", UpdateMinor},
		{"major with minor", "3.75.0", "4.1.0", UpdateMajor},
		{"two-segment current", "1.0", "1.1.0", UpdateMinor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := mustVersion(t, tt.current)
			latest := mustVersion(t, tt.latest)
			got := classifyUpdate(current, latest)
			if got != tt.want {
				t.Errorf("classifyUpdate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestShouldFilterProvider(t *testing.T) {
	tests := []struct {
		name     string
		filter   []string
		provider string
		want     bool
	}{
		{"no filter", nil, "azurerm", false},
		{"empty filter", []string{}, "azurerm", false},
		{"in filter", []string{"azurerm", "aws"}, "azurerm", false},
		{"not in filter", []string{"aws", "google"}, "azurerm", true},
		{"case insensitive", []string{"AzureRM"}, "azurerm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(Options{ProviderFilter: tt.filter})
			got := r.shouldFilterProvider(tt.provider)
			if got != tt.want {
				t.Errorf("shouldFilterProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

func TestNewResolverDefaults(t *testing.T) {
	r := New(Options{})
	if r.opts.RegistryURL != "https://registry.terraform.io" {
		t.Errorf("default RegistryURL = %q, want %q", r.opts.RegistryURL, "https://registry.terraform.io")
	}
	if r.opts.MaxConcurrency != 10 {
		t.Errorf("default MaxConcurrency = %d, want 10", r.opts.MaxConcurrency)
	}
}

func TestNewResolverCustom(t *testing.T) {
	r := New(Options{
		RegistryURL:    "https://custom.registry.io",
		MaxConcurrency: 5,
	})
	if r.opts.RegistryURL != "https://custom.registry.io" {
		t.Errorf("RegistryURL = %q, want %q", r.opts.RegistryURL, "https://custom.registry.io")
	}
	if r.opts.MaxConcurrency != 5 {
		t.Errorf("MaxConcurrency = %d, want 5", r.opts.MaxConcurrency)
	}
}

func TestBuildResolvedNoVersions(t *testing.T) {
	r := New(Options{})
	result, err := r.buildResolved("test", "src", "1.0.0", "main.tf", 1, false, false, "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty versions, got %+v", result)
	}
}

func TestBuildResolvedSameVersion(t *testing.T) {
	r := New(Options{})
	versions := []*goversion.Version{mustVersion(t, "1.0.0")}
	result, err := r.buildResolved("test", "src", "1.0.0", "main.tf", 1, false, false, "", "", versions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil when current equals latest, got %+v", result)
	}
}

func TestBuildResolvedWithUpdate(t *testing.T) {
	r := New(Options{})
	versions := []*goversion.Version{
		mustVersion(t, "1.0.0"),
		mustVersion(t, "1.1.0"),
		mustVersion(t, "1.2.0"),
	}
	result, err := r.buildResolved("test", "src", "~> 1.0.0", "main.tf", 1, true, false, "registry", "", versions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Name != "test" {
		t.Errorf("Name = %q, want %q", result.Name, "test")
	}
	if result.IsModule != true {
		t.Error("expected IsModule to be true")
	}
	if result.UpdateType != UpdateMinor {
		t.Errorf("UpdateType = %v, want %v", result.UpdateType, UpdateMinor)
	}
	if result.Latest.String() != "1.2.0" {
		t.Errorf("Latest = %q, want %q", result.Latest.String(), "1.2.0")
	}
}

func TestCacheGetSetVersions(t *testing.T) {
	cache := NewCache()

	// Should miss on empty cache
	_, ok := cache.GetVersions("key1")
	if ok {
		t.Error("expected cache miss for empty cache")
	}

	// Set and get
	versions := []*goversion.Version{mustVersion(t, "1.0.0"), mustVersion(t, "2.0.0")}
	cache.SetVersions("key1", versions)

	got, ok := cache.GetVersions("key1")
	if !ok {
		t.Fatal("expected cache hit after set")
	}
	if len(got) != 2 {
		t.Errorf("expected 2 cached versions, got %d", len(got))
	}
}

func TestCacheMiss(t *testing.T) {
	cache := NewCache()
	cache.SetVersions("key1", []*goversion.Version{mustVersion(t, "1.0.0")})

	_, ok := cache.GetVersions("key2")
	if ok {
		t.Error("expected cache miss for different key")
	}
}

func TestUpdateTypeString(t *testing.T) {
	tests := []struct {
		ut   UpdateType
		want string
	}{
		{UpdateNone, "NONE"},
		{UpdatePatch, "PATCH"},
		{UpdateMinor, "MINOR"},
		{UpdateMajor, "MAJOR"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ut.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
