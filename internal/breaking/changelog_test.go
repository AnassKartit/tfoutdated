package breaking

import (
	"testing"
)

// sampleChangelog uses line formats that match the regex patterns in changelog.go:
// - removedPattern expects: removed/deleted/dropped ... `word`
// - renamedPattern expects: renamed/replaced/changed ... `word1` ... to/with/by ... `word2`
// - requiredPattern expects: now required/is required/must be set ... `word`
// - breakingSectionRe must also match the line (BREAKING CHANGE|breaking|removed|deprecated)
const sampleChangelog = `# Changelog

## 4.0.0

BREAKING CHANGES:

* removed ` + "`azurerm_app_service`" + ` in favour of ` + "`azurerm_linux_web_app`" + ` and ` + "`azurerm_windows_web_app`" + `
* renamed ` + "`old_attr`" + ` to ` + "`new_attr`" + ` - breaking change
* is now required ` + "`required_field`" + ` on ` + "`azurerm_storage_account`" + ` - breaking

## 3.116.0

FEATURES:

* New resource: ` + "`azurerm_new_thing`" + `

## 3.115.0

BUG FIXES:

* Fixed a crash in ` + "`azurerm_kubernetes_cluster`" + `

## 3.100.0

BREAKING CHANGES:

* removed ` + "`deprecated_resource`" + ` from the provider
`

func TestSplitByVersionHeaders(t *testing.T) {
	sections := splitByVersionHeaders(sampleChangelog)
	if len(sections) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(sections))
	}

	expectedVersions := []string{"4.0.0", "3.116.0", "3.115.0", "3.100.0"}
	for i, sec := range sections {
		if sec.version != expectedVersions[i] {
			t.Errorf("section %d version = %q, want %q", i, sec.version, expectedVersions[i])
		}
	}
}

func TestSplitByVersionHeadersWithVPrefix(t *testing.T) {
	content := `## v2.1.0

Some changes

## v2.0.0

Breaking stuff
`
	sections := splitByVersionHeaders(content)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].version != "2.1.0" {
		t.Errorf("version = %q, want %q", sections[0].version, "2.1.0")
	}
}

func TestSplitByVersionHeadersEmpty(t *testing.T) {
	sections := splitByVersionHeaders("no version headers here")
	if len(sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(sections))
	}
}

func TestParseChangelogBreakingChanges(t *testing.T) {
	changes := ParseChangelog("azurerm", sampleChangelog)
	if len(changes) == 0 {
		t.Fatal("expected breaking changes to be parsed, got none")
	}

	foundRemoved := false
	foundRenamed := false
	foundRequired := false

	for _, bc := range changes {
		if bc.Provider != "azurerm" {
			t.Errorf("provider = %q, want %q", bc.Provider, "azurerm")
		}
		switch bc.Kind {
		case AttributeRemoved:
			foundRemoved = true
			if bc.Version != "4.0.0" && bc.Version != "3.100.0" {
				t.Errorf("removed change version = %q, want 4.0.0 or 3.100.0", bc.Version)
			}
		case AttributeRenamed:
			foundRenamed = true
			if bc.Version != "4.0.0" {
				t.Errorf("renamed change version = %q, want %q", bc.Version, "4.0.0")
			}
			if bc.OldValue != "old_attr" {
				t.Errorf("renamed OldValue = %q, want %q", bc.OldValue, "old_attr")
			}
			if bc.NewValue != "new_attr" {
				t.Errorf("renamed NewValue = %q, want %q", bc.NewValue, "new_attr")
			}
		case RequiredAdded:
			foundRequired = true
			if bc.Version != "4.0.0" {
				t.Errorf("required change version = %q, want %q", bc.Version, "4.0.0")
			}
		}
	}

	if !foundRemoved {
		t.Error("expected to find an AttributeRemoved change")
	}
	if !foundRenamed {
		t.Error("expected to find an AttributeRenamed change")
	}
	if !foundRequired {
		t.Error("expected to find a RequiredAdded change")
	}
}

func TestParseChangelogResourceTypeExtraction(t *testing.T) {
	changes := ParseChangelog("azurerm", sampleChangelog)

	foundAppService := false
	foundStorageAccount := false

	for _, bc := range changes {
		if bc.ResourceType == "azurerm_app_service" {
			foundAppService = true
		}
		if bc.ResourceType == "azurerm_storage_account" {
			foundStorageAccount = true
		}
	}

	if !foundAppService {
		t.Error("expected resource type azurerm_app_service to be extracted")
	}
	if !foundStorageAccount {
		t.Error("expected resource type azurerm_storage_account to be extracted")
	}
}

func TestParseChangelogIgnoresNonBreakingSections(t *testing.T) {
	changelog := `## 1.0.0

FEATURES:

* New resource: ` + "`aws_new_thing`" + `

BUG FIXES:

* Fixed issue in ` + "`aws_instance`" + `
`
	changes := ParseChangelog("aws", changelog)
	if len(changes) != 0 {
		t.Errorf("expected 0 breaking changes from non-breaking section, got %d", len(changes))
	}
}

func TestParseChangelogSeverity(t *testing.T) {
	changes := ParseChangelog("azurerm", sampleChangelog)
	for _, bc := range changes {
		if bc.Severity != SeverityBreaking {
			t.Errorf("expected all parsed changes to have SeverityBreaking, got %v for %q", bc.Severity, bc.Description)
		}
	}
}

func TestParseLineRenamed(t *testing.T) {
	// renamedPattern: renamed/replaced/changed ... `word1` ... to/with/by ... `word2`
	line := "renamed `old_name` to `new_name` - breaking change"
	bc := parseLine("azurerm", "4.0.0", line)
	if bc == nil {
		t.Fatal("expected parseLine to return a BreakingChange")
	}
	if bc.Kind != AttributeRenamed {
		t.Errorf("kind = %v, want AttributeRenamed", bc.Kind)
	}
	if bc.OldValue != "old_name" {
		t.Errorf("OldValue = %q, want %q", bc.OldValue, "old_name")
	}
	if bc.NewValue != "new_name" {
		t.Errorf("NewValue = %q, want %q", bc.NewValue, "new_name")
	}
}

func TestParseLineRemoved(t *testing.T) {
	// removedPattern: removed/deleted/dropped ... `word`
	line := "removed `some_attr` from the resource - breaking"
	bc := parseLine("azurerm", "3.0.0", line)
	if bc == nil {
		t.Fatal("expected parseLine to return a BreakingChange")
	}
	if bc.Kind != AttributeRemoved {
		t.Errorf("kind = %v, want AttributeRemoved", bc.Kind)
	}
	if bc.Attribute != "some_attr" {
		t.Errorf("Attribute = %q, want %q", bc.Attribute, "some_attr")
	}
}

func TestParseLineRequired(t *testing.T) {
	// requiredPattern: "now required|is required|must be set" ... `word`
	line := "is now required `field_name` on the resource - breaking"
	bc := parseLine("azurerm", "4.0.0", line)
	if bc == nil {
		t.Fatal("expected parseLine to return a BreakingChange")
	}
	if bc.Kind != RequiredAdded {
		t.Errorf("kind = %v, want RequiredAdded", bc.Kind)
	}
	if bc.Attribute != "field_name" {
		t.Errorf("Attribute = %q, want %q", bc.Attribute, "field_name")
	}
}

func TestParseLineRemovedWithResourceType(t *testing.T) {
	// Tests that resourcePattern extracts the resource type
	line := "removed `azurerm_app_service` from the provider - breaking"
	bc := parseLine("azurerm", "4.0.0", line)
	if bc == nil {
		t.Fatal("expected parseLine to return a BreakingChange")
	}
	if bc.ResourceType != "azurerm_app_service" {
		t.Errorf("ResourceType = %q, want %q", bc.ResourceType, "azurerm_app_service")
	}
}

func TestParseLineNoMatch(t *testing.T) {
	line := "* Some general improvement was made"
	bc := parseLine("azurerm", "4.0.0", line)
	if bc != nil {
		t.Errorf("expected nil for non-matching line, got %+v", bc)
	}
}

func TestParseLineBehaviorChangedWithResource(t *testing.T) {
	// breakingSectionRe matches + resourcePattern matches + no specific pattern
	// -> falls through to BehaviorChanged
	line := "deprecated `azurerm_virtual_network` behavior"
	bc := parseLine("azurerm", "4.0.0", line)
	if bc == nil {
		t.Fatal("expected parseLine to return a BreakingChange")
	}
	if bc.Kind != BehaviorChanged {
		t.Errorf("kind = %v, want BehaviorChanged", bc.Kind)
	}
	if bc.ResourceType != "azurerm_virtual_network" {
		t.Errorf("ResourceType = %q, want %q", bc.ResourceType, "azurerm_virtual_network")
	}
}

// --- Module-specific changelog parser tests ---

const sampleModuleChangelog = `# Changelog

## v0.11.0

BREAKING CHANGES:

* removed the variable ` + "`resource_group_name`" + `
* variable ` + "`subscription_id`" + ` removed
* variable ` + "`old_var`" + ` renamed to ` + "`new_var`" + `

## v0.10.0

FEATURES:

* Added new subnet configuration option

## v0.7.0

BREAKING CHANGES:

* Module now uses the azapi provider internally, migrated from azurerm provider
* ` + "`admin_password`" + ` is now required

## v0.5.0

CHANGES:

* deprecated ` + "`legacy_setting`" + ` in favor of new config
`

func TestParseModuleChangelogVariableRemoved(t *testing.T) {
	changes := ParseModuleChangelog("Azure/avm-res-network-virtualnetwork/azurerm", sampleModuleChangelog)

	foundResourceGroupName := false
	foundSubscriptionID := false
	for _, bc := range changes {
		if bc.Kind == VariableRemoved && bc.Attribute == "resource_group_name" {
			foundResourceGroupName = true
			if bc.Version != "0.11.0" {
				t.Errorf("version = %q, want %q", bc.Version, "0.11.0")
			}
			if !bc.IsModule {
				t.Error("expected IsModule = true")
			}
		}
		if bc.Kind == VariableRemoved && bc.Attribute == "subscription_id" {
			foundSubscriptionID = true
		}
	}

	if !foundResourceGroupName {
		t.Error("expected to find VariableRemoved for resource_group_name")
	}
	if !foundSubscriptionID {
		t.Error("expected to find VariableRemoved for subscription_id")
	}
}

func TestParseModuleChangelogVariableRenamed(t *testing.T) {
	changes := ParseModuleChangelog("Azure/avm-res-network-virtualnetwork/azurerm", sampleModuleChangelog)

	found := false
	for _, bc := range changes {
		if bc.Kind == VariableRenamed && bc.OldValue == "old_var" && bc.NewValue == "new_var" {
			found = true
			if bc.Version != "0.11.0" {
				t.Errorf("version = %q, want %q", bc.Version, "0.11.0")
			}
		}
	}

	if !found {
		t.Error("expected to find VariableRenamed for old_var -> new_var")
	}
}

func TestParseModuleChangelogProviderMigrated(t *testing.T) {
	changes := ParseModuleChangelog("Azure/avm-res-network-bastionhost/azurerm", sampleModuleChangelog)

	found := false
	for _, bc := range changes {
		if bc.Kind == ProviderMigrated && bc.Version == "0.7.0" {
			found = true
			if bc.Severity != SeverityBreaking {
				t.Errorf("severity = %v, want SeverityBreaking", bc.Severity)
			}
		}
	}

	if !found {
		t.Error("expected to find ProviderMigrated change in v0.7.0")
	}
}

func TestParseModuleChangelogRequiredAdded(t *testing.T) {
	changes := ParseModuleChangelog("Azure/avm-res-compute-virtualmachine/azurerm", sampleModuleChangelog)

	found := false
	for _, bc := range changes {
		if bc.Kind == RequiredAdded && bc.Attribute == "admin_password" {
			found = true
			if bc.Version != "0.7.0" {
				t.Errorf("version = %q, want %q", bc.Version, "0.7.0")
			}
		}
	}

	if !found {
		t.Error("expected to find RequiredAdded for admin_password")
	}
}

func TestParseModuleChangelogDeprecated(t *testing.T) {
	changes := ParseModuleChangelog("Azure/avm-res-test/azurerm", sampleModuleChangelog)

	found := false
	for _, bc := range changes {
		if bc.Attribute == "legacy_setting" && bc.Version == "0.5.0" {
			found = true
			if bc.Severity != SeverityWarning {
				t.Errorf("severity = %v, want SeverityWarning", bc.Severity)
			}
		}
	}

	if !found {
		t.Error("expected to find deprecation notice for legacy_setting")
	}
}

func TestParseModuleChangelogIgnoresFeatureSections(t *testing.T) {
	changelog := `## v1.0.0

FEATURES:

* Added new cool feature
* New output available
`
	changes := ParseModuleChangelog("Azure/avm-res-test/azurerm", changelog)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes from feature-only section, got %d", len(changes))
	}
}

func TestParseModuleLineProviderMigratedVariants(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"uses azapi provider", "Module now uses the azapi provider", true},
		{"migrated from azurerm", "migrated from azurerm to azapi provider", true},
		{"switched to azapi", "switched to the azapi provider for internal resources", true},
		{"moved to azapi", "moved to azapi provider", true},
		{"unrelated line", "Updated documentation links", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := parseModuleLine("test/module/azurerm", "1.0.0", tt.line)
			if tt.want && bc == nil {
				t.Errorf("expected parseModuleLine to return a change for %q", tt.line)
			}
			if tt.want && bc != nil && bc.Kind != ProviderMigrated {
				t.Errorf("expected ProviderMigrated, got %v for %q", bc.Kind, tt.line)
			}
			if !tt.want && bc != nil && bc.Kind == ProviderMigrated {
				t.Errorf("did not expect ProviderMigrated for %q", tt.line)
			}
		})
	}
}

func TestParseModuleLineVariableRemovedVariants(t *testing.T) {
	tests := []struct {
		name string
		line string
		attr string
	}{
		{"removed the variable", "removed the variable `foo_bar`", "foo_bar"},
		{"variable X removed", "variable `baz_qux` removed from module", "baz_qux"},
		{"deleted variable", "deleted variable `old_thing`", "old_thing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := parseModuleLine("test/mod/azurerm", "1.0.0", tt.line)
			if bc == nil {
				t.Fatalf("expected a change for %q", tt.line)
			}
			if bc.Kind != VariableRemoved {
				t.Errorf("kind = %v, want VariableRemoved for %q", bc.Kind, tt.line)
			}
			if bc.Attribute != tt.attr {
				t.Errorf("Attribute = %q, want %q", bc.Attribute, tt.attr)
			}
		})
	}
}

func TestParseModuleLineVariableRenamedVariants(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		oldVal string
		newVal string
	}{
		{"variable X renamed to Y", "variable `old_name` renamed to `new_name`", "old_name", "new_name"},
		{"renamed variable X to Y", "renamed variable `alpha` to `beta`", "alpha", "beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := parseModuleLine("test/mod/azurerm", "1.0.0", tt.line)
			if bc == nil {
				t.Fatalf("expected a change for %q", tt.line)
			}
			if bc.Kind != VariableRenamed {
				t.Errorf("kind = %v, want VariableRenamed for %q", bc.Kind, tt.line)
			}
			if bc.OldValue != tt.oldVal {
				t.Errorf("OldValue = %q, want %q", bc.OldValue, tt.oldVal)
			}
			if bc.NewValue != tt.newVal {
				t.Errorf("NewValue = %q, want %q", bc.NewValue, tt.newVal)
			}
		})
	}
}

func TestParseModuleLineDeprecatedVariants(t *testing.T) {
	tests := []struct {
		name string
		line string
		attr string
	}{
		{"X deprecated", "`some_var` deprecated in this release", "some_var"},
		{"X is deprecated", "`other_var` is deprecated", "other_var"},
		{"deprecated X", "deprecated `old_setting` in favor of new one", "old_setting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := parseModuleLine("test/mod/azurerm", "1.0.0", tt.line)
			if bc == nil {
				t.Fatalf("expected a change for %q", tt.line)
			}
			if bc.Attribute != tt.attr {
				t.Errorf("Attribute = %q, want %q", bc.Attribute, tt.attr)
			}
			if bc.Severity != SeverityWarning {
				t.Errorf("Severity = %v, want SeverityWarning", bc.Severity)
			}
		})
	}
}

func TestParseModuleLineRequiredAddedVariants(t *testing.T) {
	tests := []struct {
		name string
		line string
		attr string
	}{
		{"X is now required", "`field_a` is now required", "field_a"},
		{"X is required", "`field_b` is required after upgrade", "field_b"},
		{"added required variable", "added required variable `field_c`", "field_c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := parseModuleLine("test/mod/azurerm", "1.0.0", tt.line)
			if bc == nil {
				t.Fatalf("expected a change for %q", tt.line)
			}
			if bc.Kind != RequiredAdded {
				t.Errorf("kind = %v, want RequiredAdded for %q", bc.Kind, tt.line)
			}
			if bc.Attribute != tt.attr {
				t.Errorf("Attribute = %q, want %q", bc.Attribute, tt.attr)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "c"); got != "c" {
		t.Errorf("got %q, want %q", got, "c")
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("got %q, want %q", got, "a")
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
