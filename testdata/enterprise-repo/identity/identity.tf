# Managed Identities
resource "azurerm_user_assigned_identity" "app" {
  name                = "id-app-prod"
  location            = azurerm_resource_group.identity.location
  resource_group_name = azurerm_resource_group.identity.name
}

resource "azurerm_user_assigned_identity" "aks" {
  name                = "id-aks-prod"
  location            = azurerm_resource_group.identity.location
  resource_group_name = azurerm_resource_group.identity.name
}

resource "azurerm_user_assigned_identity" "data_pipeline" {
  name                = "id-data-pipeline-prod"
  location            = azurerm_resource_group.identity.location
  resource_group_name = azurerm_resource_group.identity.name
}

# Azure AD Groups
resource "azuread_group" "platform_admins" {
  display_name     = "grp-platform-admins"
  security_enabled = true
  mail_enabled     = false
}

resource "azuread_group" "developers" {
  display_name     = "grp-developers"
  security_enabled = true
  mail_enabled     = false
}

resource "azuread_group" "data_engineers" {
  display_name     = "grp-data-engineers"
  security_enabled = true
  mail_enabled     = false
}

# Azure AD Application (demonstrates application_id -> client_id breaking change)
resource "azuread_application" "api" {
  display_name = "enterprise-api-prod"

  required_resource_access {
    resource_app_id = "00000003-0000-0000-c000-000000000000" # Microsoft Graph

    resource_access {
      id   = "e1fe6dd8-ba31-4d61-89e7-88639da4683d" # User.Read
      type = "Scope"
    }
  }

  web {
    redirect_uris = ["https://api.enterprise.com/auth/callback"]
  }
}

resource "azuread_service_principal" "api" {
  application_id = azuread_application.api.application_id
}

resource "azuread_application" "worker" {
  display_name = "enterprise-worker-prod"
}

resource "azuread_service_principal" "worker" {
  application_id = azuread_application.worker.application_id
}

# RBAC Role Assignments
resource "azurerm_role_assignment" "platform_admins_contributor" {
  scope                = data.azurerm_subscription.current.id
  role_definition_name = "Contributor"
  principal_id         = azuread_group.platform_admins.object_id
}

resource "azurerm_role_assignment" "developers_reader" {
  scope                = azurerm_resource_group.main.id
  role_definition_name = "Reader"
  principal_id         = azuread_group.developers.object_id
}

resource "azurerm_role_assignment" "data_engineers_storage" {
  scope                = azurerm_storage_account.data_lake.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azuread_group.data_engineers.object_id
}

resource "azurerm_role_assignment" "app_identity_kv" {
  scope                = azurerm_key_vault.secondary.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = azurerm_user_assigned_identity.app.principal_id
}

resource "azurerm_role_assignment" "aks_identity_acr" {
  scope                = module.container_registry.resource_id
  role_definition_name = "AcrPull"
  principal_id         = azurerm_user_assigned_identity.aks.principal_id
}
