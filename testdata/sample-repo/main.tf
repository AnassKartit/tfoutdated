terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.75.0"
    }
    azuread = {
      source  = "hashicorp/azuread"
      version = "~> 2.47.0"
    }
    azapi = {
      source  = "Azure/azapi"
      version = "~> 1.9.0"
    }
  }
}

provider "azurerm" {
  features {}
}

provider "azuread" {}

resource "azurerm_resource_group" "main" {
  name     = "rg-tfoutdated-demo"
  location = "westeurope"
}

resource "azurerm_virtual_network" "main" {
  name                = "vnet-main"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_subnet" "main" {
  name                 = "snet-main"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_storage_account" "main" {
  name                     = "sttfoutdateddemo"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
}

resource "azurerm_kubernetes_cluster" "main" {
  name                = "aks-demo"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  dns_prefix          = "aksdemo"

  default_node_pool {
    name       = "default"
    node_count = 1
    vm_size    = "Standard_D2_v2"
  }

  identity {
    type = "SystemAssigned"
  }
}

resource "azuread_application" "main" {
  display_name = "tfoutdated-demo-app"
}

resource "azuread_service_principal" "main" {
  application_id = azuread_application.main.application_id
}
