resource "azurerm_storage_account" "main" {
  name                     = "stenterpriseprod"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "GRS"

  # Note: min_tls_version default changes in v4
  # min_tls_version = "TLS1_2"

  blob_properties {
    versioning_enabled  = true
    change_feed_enabled = true

    delete_retention_policy {
      days = 30
    }
  }

  tags = {
    environment = "production"
  }
}

resource "azurerm_storage_account" "functions" {
  name                     = "stfuncprod"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_storage_account" "data_lake" {
  name                     = "stdatalakeprod"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "GRS"
  account_kind             = "StorageV2"
  is_hns_enabled           = true

  network_rules {
    default_action             = "Deny"
    bypass                     = ["AzureServices"]
    virtual_network_subnet_ids = [azurerm_subnet.data.id]
  }
}

module "storage_avm" {
  source  = "Azure/avm-res-storage-storageaccount/azurerm"
  version = "~> 0.1.0"

  name                = "stavmprod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}
