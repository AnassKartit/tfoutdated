# Azure Verified Modules
module "virtual_machine" {
  source  = "Azure/avm-res-compute-virtualmachine/azurerm"
  version = "~> 0.6.0"

  name                = "vm-demo"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  zone = 1
}

module "key_vault" {
  source  = "Azure/avm-res-keyvault-vault/azurerm"
  version = "~> 0.5.0"

  name                = "kv-tfoutdated-demo"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  tenant_id           = data.azurerm_client_config.current.tenant_id
}

module "container_registry" {
  source  = "Azure/avm-res-containerregistry-registry/azurerm"
  version = "~> 0.1.0"

  name                = "crtfoutdateddemo"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

# Standard Azure modules (non-AVM)
module "naming" {
  source  = "Azure/naming/azurerm"
  version = "~> 0.3.0"

  prefix = ["demo"]
}

module "network" {
  source  = "Azure/network/azurerm"
  version = "~> 5.2.0"

  resource_group_name = azurerm_resource_group.main.name
  vnet_name           = "vnet-modules"
  address_space       = "10.1.0.0/16"
}

data "azurerm_client_config" "current" {}
