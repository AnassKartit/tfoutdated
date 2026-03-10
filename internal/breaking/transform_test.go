package breaking

import (
	"strings"
	"testing"
)

func TestApplyTransformNil(t *testing.T) {
	input := `resource "azurerm_app_service" "main" {
  name = "test"
}`
	result, changed := ApplyTransform(input, nil)
	if result != input {
		t.Error("expected no change for nil transform")
	}
	if changed != 0 {
		t.Errorf("expected 0 lines changed, got %d", changed)
	}
}

func TestApplyTransformRenameResource(t *testing.T) {
	input := `resource "azurerm_app_service" "main" {
  name = "test"
}`
	tr := &Transform{
		RenameResource: "azurerm_linux_web_app",
	}
	result, changed := ApplyTransform(input, tr)
	if !strings.Contains(result, `resource "azurerm_linux_web_app" "main"`) {
		t.Errorf("expected resource rename, got:\n%s", result)
	}
	if changed != 1 {
		t.Errorf("expected 1 line changed, got %d", changed)
	}
}

func TestApplyTransformRenameAttrs(t *testing.T) {
	input := `resource "azuread_service_principal" "main" {
  application_id = azuread_application.main.application_id
}`
	tr := &Transform{
		RenameAttrs: map[string]string{"application_id": "client_id"},
	}
	result, changed := ApplyTransform(input, tr)
	// The attribute name (left of =) should be renamed
	if !strings.Contains(result, "client_id =") {
		t.Errorf("expected attribute name rename, got:\n%s", result)
	}
	// The first occurrence on the line should be renamed (the attr name)
	lines := strings.Split(result, "\n")
	attrLine := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(attrLine, "client_id") {
		t.Errorf("expected line to start with client_id, got: %s", attrLine)
	}
	if changed != 1 {
		t.Errorf("expected 1 line changed, got %d", changed)
	}
}

func TestApplyTransformRemoveAttrs(t *testing.T) {
	input := `resource "azurerm_app_service" "main" {
  name                     = "test"
  dotnet_framework_version = "v6.0"
  always_on                = true
}`
	tr := &Transform{
		RemoveAttrs: []string{"dotnet_framework_version"},
	}
	result, changed := ApplyTransform(input, tr)
	if !strings.Contains(result, "# REMOVED: dotnet_framework_version") {
		t.Errorf("expected attribute to be commented out, got:\n%s", result)
	}
	if changed != 1 {
		t.Errorf("expected 1 line changed, got %d", changed)
	}
}

func TestApplyTransformRemoveBlockAttrs(t *testing.T) {
	input := `resource "azurerm_app_service_plan" "main" {
  name = "test"

  sku {
    tier = "Standard"
    size = "S1"
  }
}`
	tr := &Transform{
		RemoveAttrs: []string{"sku"},
	}
	result, changed := ApplyTransform(input, tr)
	if !strings.Contains(result, "# REMOVED: sku {") {
		t.Errorf("expected sku block to be commented out, got:\n%s", result)
	}
	if !strings.Contains(result, "# REMOVED: tier") {
		t.Errorf("expected sku block contents to be commented out, got:\n%s", result)
	}
	if changed < 3 {
		t.Errorf("expected at least 3 lines changed, got %d", changed)
	}
}

func TestApplyTransformAddAttrs(t *testing.T) {
	input := `resource "azurerm_service_plan" "main" {
  name = "test"
}`
	tr := &Transform{
		AddAttrs: map[string]string{"os_type": `"Linux"`},
	}
	result, changed := ApplyTransform(input, tr)
	if !strings.Contains(result, `os_type = "Linux"`) {
		t.Errorf("expected added attribute, got:\n%s", result)
	}
	if changed != 1 {
		t.Errorf("expected 1 line changed, got %d", changed)
	}
}

func TestApplyTransformCombined(t *testing.T) {
	input := `resource "azurerm_app_service_plan" "legacy" {
  name                = "asp-legacy-api-prod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  sku {
    tier = "Standard"
    size = "S2"
  }
}`
	tr := &Transform{
		RenameResource: "azurerm_service_plan",
		RemoveAttrs:    []string{"sku"},
		AddAttrs:       map[string]string{"os_type": `"Linux"`, "sku_name": `"S1"`},
	}
	result, changed := ApplyTransform(input, tr)

	if !strings.Contains(result, `resource "azurerm_service_plan" "legacy"`) {
		t.Errorf("expected resource rename, got:\n%s", result)
	}
	if !strings.Contains(result, "# REMOVED: sku") {
		t.Errorf("expected sku block removal, got:\n%s", result)
	}
	if !strings.Contains(result, `os_type = "Linux"`) {
		t.Errorf("expected os_type added, got:\n%s", result)
	}
	if !strings.Contains(result, `sku_name = "S1"`) {
		t.Errorf("expected sku_name added, got:\n%s", result)
	}
	if changed < 4 {
		t.Errorf("expected at least 4 lines changed, got %d", changed)
	}
}
