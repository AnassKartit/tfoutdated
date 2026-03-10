terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.10.0"
    }
    azapi = {
      source  = "azure/azapi"
      version = "~> 2.0.0"
    }
  }
}

# VNet - service_endpoints drift issue (v0.13.0 → v0.14.0+ = azapi subnets)
# Real issues: Azure/terraform-azurerm-avm-res-network-virtualnetwork#50, #39
module "vnet_main" {
  source  = "Azure/avm-res-network-virtualnetwork/azurerm"
  version = "0.13.0"

  name                = "vnet-prod-001"
  resource_group_name = "rg-networking"
  location            = "westeurope"
  address_space       = ["10.0.0.0/16"]

  subnets = {
    aks = {
      name             = "snet-aks"
      address_prefixes = ["10.0.1.0/24"]
      service_endpoints = ["Microsoft.Storage", "Microsoft.KeyVault"]
    }
    apps = {
      name             = "snet-apps"
      address_prefixes = ["10.0.2.0/24"]
      service_endpoints = ["Microsoft.Sql"]
      delegation = {
        name = "delegation"
        service_delegation = {
          name    = "Microsoft.Web/serverFarms"
          actions = ["Microsoft.Network/virtualNetworks/subnets/action"]
        }
      }
    }
    bastion = {
      name             = "AzureBastionSubnet"
      address_prefixes = ["10.0.255.0/24"]
    }
  }
}

# NSG - output renamed (v0.5.0 → v0.5.1 = output 'resource' → 'security_rules')
# Real issue: Azure/terraform-azurerm-avm-res-network-networksecuritygroup#121
module "nsg_aks" {
  source  = "Azure/avm-res-network-networksecuritygroup/azurerm"
  version = "0.5.0"

  name                = "nsg-aks"
  resource_group_name = "rg-networking"
  location            = "westeurope"

  security_rules = {
    allow_https = {
      name                       = "AllowHTTPS"
      priority                   = 100
      direction                  = "Inbound"
      access                     = "Allow"
      protocol                   = "Tcp"
      source_port_range          = "*"
      destination_port_range     = "443"
      source_address_prefix      = "*"
      destination_address_prefix = "*"
    }
    deny_all = {
      name                       = "DenyAll"
      priority                   = 4096
      direction                  = "Inbound"
      access                     = "Deny"
      protocol                   = "*"
      source_port_range          = "*"
      destination_port_range     = "*"
      source_address_prefix      = "*"
      destination_address_prefix = "*"
    }
  }
}

# NSG for apps subnet
module "nsg_apps" {
  source  = "Azure/avm-res-network-networksecuritygroup/azurerm"
  version = "0.4.0"

  name                = "nsg-apps"
  resource_group_name = "rg-networking"
  location            = "westeurope"

  security_rules = {
    allow_app_gw = {
      name                       = "AllowAppGateway"
      priority                   = 100
      direction                  = "Inbound"
      access                     = "Allow"
      protocol                   = "Tcp"
      source_port_range          = "*"
      destination_port_range     = "8080"
      source_address_prefix      = "10.0.3.0/24"
      destination_address_prefix = "*"
    }
  }
}

# Bastion Host
module "bastion" {
  source  = "Azure/avm-res-network-bastionhost/azurerm"
  version = "0.6.0"

  name                = "bas-prod"
  resource_group_name = "rg-networking"
  location            = "westeurope"
}
