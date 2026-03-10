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

  backend "azurerm" {
    resource_group_name  = "rg-terraform-state"
    storage_account_name = "stterraformstate"
    container_name       = "tfstate"
    key                  = "enterprise.tfstate"
  }
}

provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy = true
    }
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}

provider "azuread" {}

# Resource Groups
resource "azurerm_resource_group" "main" {
  name     = "rg-enterprise-prod"
  location = "westeurope"

  tags = {
    environment = "production"
    managed_by  = "terraform"
  }
}

resource "azurerm_resource_group" "networking" {
  name     = "rg-networking-prod"
  location = "westeurope"
}

resource "azurerm_resource_group" "identity" {
  name     = "rg-identity-prod"
  location = "westeurope"
}

resource "azurerm_resource_group" "security" {
  name     = "rg-security-prod"
  location = "westeurope"
}

data "azurerm_client_config" "current" {}
data "azurerm_subscription" "current" {}
