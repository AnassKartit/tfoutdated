module "key_vault" {
  source  = "Azure/avm-res-keyvault-vault/azurerm"
  version = "~> 0.5.0"

  name                = "kv-enterprise-prod"
  location            = azurerm_resource_group.security.location
  resource_group_name = azurerm_resource_group.security.name
  tenant_id           = data.azurerm_client_config.current.tenant_id
}

resource "azurerm_key_vault" "secondary" {
  name                = "kv-enterprise-dr"
  location            = "northeurope"
  resource_group_name = azurerm_resource_group.security.name
  tenant_id           = data.azurerm_client_config.current.tenant_id
  sku_name            = "standard"

  purge_protection_enabled   = true
  soft_delete_retention_days = 90

  network_acls {
    default_action = "Deny"
    bypass         = "AzureServices"
  }
}

resource "azurerm_key_vault_access_policy" "app_identity" {
  key_vault_id = azurerm_key_vault.secondary.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_user_assigned_identity.app.principal_id

  secret_permissions = [
    "Get",
    "List",
  ]

  key_permissions = [
    "Get",
    "WrapKey",
    "UnwrapKey",
  ]
}

resource "azurerm_key_vault_access_policy" "aks_identity" {
  key_vault_id = azurerm_key_vault.secondary.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_user_assigned_identity.aks.principal_id

  secret_permissions = [
    "Get",
    "List",
  ]
}
