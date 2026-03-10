package breaking

import (
	"testing"
)

func TestNewKnowledgeBaseNotEmpty(t *testing.T) {
	kb := NewKnowledgeBase()
	providers := kb.SupportedProviders()
	if len(providers) == 0 {
		t.Fatal("expected knowledge base to have supported providers")
	}

	// Check that the three Azure providers are registered
	provSet := make(map[string]bool)
	for _, p := range providers {
		provSet[p] = true
	}

	for _, expected := range []string{"azurerm", "azuread", "azapi"} {
		if !provSet[expected] {
			t.Errorf("expected provider %q to be in knowledge base", expected)
		}
	}
}

func TestGetChangesAzurermV3ToV4(t *testing.T) {
	kb := NewKnowledgeBase()
	changes := kb.GetChanges("azurerm", "3.75.0", "4.0.0")

	if len(changes) == 0 {
		t.Fatal("expected breaking changes for azurerm 3.75.0 -> 4.0.0, got none")
	}

	// Should include the v4 breaking changes but not the v3 ones (since from > 3.75.0)
	foundAppService := false
	for _, bc := range changes {
		if bc.Provider != "azurerm" {
			t.Errorf("provider = %q, want azurerm", bc.Provider)
		}
		if bc.ResourceType == "azurerm_app_service" && bc.Version == "4.0.0" {
			foundAppService = true
		}
	}

	if !foundAppService {
		t.Error("expected to find azurerm_app_service breaking change in v4.0.0")
	}
}

func TestGetChangesAzurermV2ToV3(t *testing.T) {
	kb := NewKnowledgeBase()
	changes := kb.GetChanges("azurerm", "2.99.0", "3.0.0")

	if len(changes) == 0 {
		t.Fatal("expected breaking changes for azurerm 2.99.0 -> 3.0.0, got none")
	}

	foundVM := false
	for _, bc := range changes {
		if bc.ResourceType == "azurerm_virtual_machine" && bc.Version == "3.0.0" {
			foundVM = true
		}
	}
	if !foundVM {
		t.Error("expected to find azurerm_virtual_machine breaking change in v3.0.0")
	}
}

func TestGetChangesAzurermV2ToV4(t *testing.T) {
	kb := NewKnowledgeBase()
	changes := kb.GetChanges("azurerm", "2.99.0", "4.0.0")

	// Should include both v3 and v4 changes
	hasV3 := false
	hasV4 := false
	for _, bc := range changes {
		if bc.Version == "3.0.0" {
			hasV3 = true
		}
		if bc.Version == "4.0.0" {
			hasV4 = true
		}
	}

	if !hasV3 {
		t.Error("expected v3.0.0 breaking changes when upgrading from 2.99.0 to 4.0.0")
	}
	if !hasV4 {
		t.Error("expected v4.0.0 breaking changes when upgrading from 2.99.0 to 4.0.0")
	}
}

func TestGetChangesAzuread(t *testing.T) {
	kb := NewKnowledgeBase()
	changes := kb.GetChanges("azuread", "2.47.0", "3.0.0")

	if len(changes) == 0 {
		t.Fatal("expected breaking changes for azuread 2.47.0 -> 3.0.0, got none")
	}

	foundClientID := false
	for _, bc := range changes {
		if bc.Provider != "azuread" {
			t.Errorf("provider = %q, want azuread", bc.Provider)
		}
		if bc.Attribute == "application_id" && bc.Kind == AttributeRenamed {
			foundClientID = true
			if !bc.AutoFixable {
				t.Error("expected application_id -> client_id to be auto-fixable")
			}
		}
	}

	if !foundClientID {
		t.Error("expected to find application_id renamed to client_id in azuread v3")
	}
}

func TestGetChangesAzapi(t *testing.T) {
	kb := NewKnowledgeBase()
	changes := kb.GetChanges("azapi", "1.0.0", "2.0.0")

	if len(changes) == 0 {
		t.Fatal("expected breaking changes for azapi 1.0.0 -> 2.0.0, got none")
	}

	foundBody := false
	for _, bc := range changes {
		if bc.Attribute == "body" && bc.Kind == TypeChanged {
			foundBody = true
		}
	}
	if !foundBody {
		t.Error("expected to find body attribute type change in azapi v2.0.0")
	}
}

func TestGetChangesVersionFiltering(t *testing.T) {
	kb := NewKnowledgeBase()

	// From version 4.0.0 to 4.0.0 should return nothing (no changes after 4.0.0 up to 4.0.0)
	// The condition is bc.Version > fromVersion && bc.Version <= toVersion
	// 4.0.0 > 4.0.0 is false, so nothing should match
	changes := kb.GetChanges("azurerm", "4.0.0", "4.0.0")
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for same version range, got %d", len(changes))
	}
}

func TestGetChangesUnknownProvider(t *testing.T) {
	kb := NewKnowledgeBase()
	changes := kb.GetChanges("unknown_provider", "1.0.0", "2.0.0")
	if changes != nil {
		t.Errorf("expected nil for unknown provider, got %v", changes)
	}
}

func TestGetChangesNarrowRange(t *testing.T) {
	kb := NewKnowledgeBase()

	// From 3.0.0 to 3.75.0 should return no changes since all registered changes
	// for azurerm are at version 3.0.0 or 4.0.0
	// Version 3.0.0 > 3.0.0 is false, so the v3 changes won't be included
	// Version 4.0.0 <= 3.75.0 is false, so the v4 changes won't be included
	changes := kb.GetChanges("azurerm", "3.0.0", "3.75.0")
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for 3.0.0 -> 3.75.0, got %d", len(changes))
	}
}

func TestSupportedProvidersContainsExpected(t *testing.T) {
	kb := NewKnowledgeBase()
	providers := kb.SupportedProviders()

	if len(providers) < 3 {
		t.Errorf("expected at least 3 supported providers, got %d", len(providers))
	}
}
