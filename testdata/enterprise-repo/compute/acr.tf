# Azure Verified Module for Container Registry
module "container_registry" {
  source  = "Azure/avm-res-containerregistry-registry/azurerm"
  version = "~> 0.1.0"

  name                = "crenterpriseprod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

# Standard naming module
module "naming" {
  source  = "Azure/naming/azurerm"
  version = "~> 0.3.0"

  prefix = ["enterprise", "prod"]
}
