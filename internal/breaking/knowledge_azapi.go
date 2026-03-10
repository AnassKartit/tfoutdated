package breaking

func (kb *KnowledgeBase) registerAzAPI() {
	kb.register("azapi", []BreakingChange{
		// azapi v1 → v2
		{
			Provider:       "azapi",
			Version:        "2.0.0",
			ResourceType:   "azapi_resource",
			Attribute:      "body",
			Kind:           TypeChanged,
			Severity:       SeverityBreaking,
			Description:    "The body attribute changed from JSON string to dynamic type. Use jsonencode() for the value.",
			MigrationGuide: "Change body = jsonencode({...}) to body = {...} (remove jsonencode wrapper).",
			OldValue:       "body (JSON string)",
			NewValue:       "body (dynamic)",
			AutoFixable:    false,
			EffortLevel:    "medium",
			BeforeSnippet: `resource "azapi_resource" "main" {
  type      = "Microsoft.Network/virtualNetworks@2021-02-01"
  name      = "my-vnet"
  parent_id = azurerm_resource_group.main.id

  body = jsonencode({
    properties = {
      addressSpace = {
        addressPrefixes = ["10.0.0.0/16"]
      }
    }
  })
}`,
			AfterSnippet: `resource "azapi_resource" "main" {
  type      = "Microsoft.Network/virtualNetworks@2021-02-01"
  name      = "my-vnet"
  parent_id = azurerm_resource_group.main.id

  # In v2, body is a dynamic type — no jsonencode() needed
  body = {
    properties = {
      addressSpace = {
        addressPrefixes = ["10.0.0.0/16"]
      }
    }
  }
}`,
			// No mechanical Transform — removing jsonencode() requires AST-level rewriting
		},
		{
			Provider:       "azapi",
			Version:        "2.0.0",
			ResourceType:   "azapi_resource",
			Attribute:      "response_export_values",
			Kind:           TypeChanged,
			Severity:       SeverityBreaking,
			Description:    "response_export_values changed from list of strings to a dynamic type.",
			MigrationGuide: "Update response_export_values to use the new dynamic syntax.",
			AutoFixable:    false,
			EffortLevel:    "medium",
			BeforeSnippet: `resource "azapi_resource" "main" {
  # ...
  response_export_values = [
    "properties.provisioningState",
    "properties.addressSpace"
  ]
}

# Access: azapi_resource.main.output.properties.provisioningState`,
			AfterSnippet: `resource "azapi_resource" "main" {
  # ...
  response_export_values = {
    state = "properties.provisioningState"
    addr  = "properties.addressSpace"
  }
}

# Access: azapi_resource.main.output.state`,
		},
	})
}
