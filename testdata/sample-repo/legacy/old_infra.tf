# This file uses older/deprecated resources to demonstrate breaking change detection

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.50.0"
    }
  }
}

resource "azurerm_app_service_plan" "legacy" {
  name                = "asp-legacy"
  location            = "westeurope"
  resource_group_name = "rg-legacy"

  sku {
    tier = "Standard"
    size = "S1"
  }
}

resource "azurerm_app_service" "legacy" {
  name                = "app-legacy"
  location            = "westeurope"
  resource_group_name = "rg-legacy"
  app_service_plan_id = azurerm_app_service_plan.legacy.id

  site_config {
    dotnet_framework_version = "v4.0"
  }
}

resource "azurerm_function_app" "legacy" {
  name                       = "func-legacy"
  location                   = "westeurope"
  resource_group_name        = "rg-legacy"
  app_service_plan_id        = azurerm_app_service_plan.legacy.id
  storage_account_name       = "stlegacy"
  storage_account_access_key = "key"
}

resource "azurerm_template_deployment" "legacy" {
  name                = "template-legacy"
  resource_group_name = "rg-legacy"

  template_body = <<DEPLOY
{
  "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
  "contentVersion": "1.0.0.0",
  "resources": []
}
DEPLOY

  deployment_mode = "Incremental"
}
