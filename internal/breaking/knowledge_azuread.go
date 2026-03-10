package breaking

func (kb *KnowledgeBase) registerAzureAD() {
	kb.register("azuread", []BreakingChange{
		// azuread v2 → v3 (Microsoft Graph migration)
		{
			Provider:       "azuread",
			Version:        "3.0.0",
			Kind:           BehaviorChanged,
			Severity:       SeverityBreaking,
			Description:    "azuread v3 completed the migration from Azure AD Graph to Microsoft Graph API.",
			MigrationGuide: "Review all azuread resources for changes in attribute names and behavior due to Microsoft Graph migration.",
			AutoFixable:    false,
			EffortLevel:    "medium",
			BeforeSnippet: `# azuread v2 used Azure AD Graph API under the hood.
# Many attribute names reflected the old API.`,
			AfterSnippet: `# azuread v3 uses Microsoft Graph API.
# Attribute names now match Microsoft Graph terminology.
# Review all azuread_* resources for renamed attributes.`,
		},
		{
			Provider:       "azuread",
			Version:        "3.0.0",
			ResourceType:   "azuread_application",
			Attribute:      "api",
			Kind:           TypeChanged,
			Severity:       SeverityBreaking,
			Description:    "The api block structure has changed. oauth2_permission_scope replaces oauth2_permissions.",
			OldValue:       "oauth2_permissions",
			NewValue:       "api.oauth2_permission_scope",
			AutoFixable:    false,
			EffortLevel:    "medium",
			BeforeSnippet: `resource "azuread_application" "main" {
  display_name = "my-app"

  oauth2_permissions {
    admin_consent_description  = "Allow access"
    admin_consent_display_name = "Access"
    value                      = "user_impersonation"
  }
}`,
			AfterSnippet: `resource "azuread_application" "main" {
  display_name = "my-app"

  api {
    oauth2_permission_scope {
      admin_consent_description  = "Allow access"
      admin_consent_display_name = "Access"
      value                      = "user_impersonation"
      id                         = "00000000-0000-0000-0000-000000000000"
    }
  }
}`,
		},
		{
			Provider:       "azuread",
			Version:        "3.0.0",
			ResourceType:   "azuread_application",
			Attribute:      "required_resource_access",
			Kind:           AttributeRenamed,
			Severity:       SeverityBreaking,
			Description:    "required_resource_access has been moved to api.required_resource_access.",
			OldValue:       "required_resource_access",
			NewValue:       "required_resource_access (nested under api block)",
			AutoFixable:    false,
			EffortLevel:    "small",
			BeforeSnippet: `resource "azuread_application" "main" {
  display_name = "my-app"

  required_resource_access {
    resource_app_id = "00000003-0000-0000-c000-000000000000"
    resource_access {
      id   = "e1fe6dd8-ba31-4d61-89e7-88639da4683d"
      type = "Scope"
    }
  }
}`,
			AfterSnippet: `resource "azuread_application" "main" {
  display_name = "my-app"

  api {
    # moved inside the api block
  }

  required_resource_access {
    resource_app_id = "00000003-0000-0000-c000-000000000000"
    resource_access {
      id   = "e1fe6dd8-ba31-4d61-89e7-88639da4683d"
      type = "Scope"
    }
  }
}`,
		},
		{
			Provider:       "azuread",
			Version:        "3.0.0",
			ResourceType:   "azuread_service_principal",
			Attribute:      "application_id",
			Kind:           AttributeRenamed,
			Severity:       SeverityBreaking,
			Description:    "application_id has been renamed to client_id across all resources.",
			OldValue:       "application_id",
			NewValue:       "client_id",
			AutoFixable:    true,
			EffortLevel:    "small",
			BeforeSnippet: `resource "azuread_service_principal" "main" {
  application_id = azuread_application.main.application_id
}`,
			AfterSnippet: `resource "azuread_service_principal" "main" {
  client_id = azuread_application.main.client_id
}`,
			Transform: &Transform{
				RenameAttrs: map[string]string{"application_id": "client_id"},
			},
		},
	})
}
