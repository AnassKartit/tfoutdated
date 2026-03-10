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

# AKS - pre-azapi migration version (v0.3.3 → v0.4.0 = full azapi rewrite)
# Real issue: Azure/terraform-azurerm-avm-res-containerservice-managedcluster#107
module "aks_cluster" {
  source  = "Azure/avm-res-containerservice-managedcluster/azurerm"
  version = "0.3.3"

  name                = "aks-prod-001"
  resource_group_name = "rg-compute"
  location            = "westeurope"

  default_node_pool = {
    name       = "system"
    vm_size    = "Standard_D4s_v5"
    node_count = 3
  }

  kubernetes_version = "1.30"
}

# AKS - private cluster broken by v0.5.0 auto-generated dns_prefix
# Real issue: Azure/terraform-azurerm-avm-res-containerservice-managedcluster#166
module "aks_private" {
  source  = "Azure/avm-res-containerservice-managedcluster/azurerm"
  version = "0.4.3"

  name                = "aks-private-001"
  resource_group_name = "rg-compute"
  location            = "westeurope"

  default_node_pool = {
    name       = "system"
    vm_size    = "Standard_D4s_v5"
    node_count = 3
  }

  kubernetes_version = "1.30"
}

# Container Apps - pre-azapi (v0.6.0 → v0.7.0 breaks custom_domains)
# Real issue: Azure/terraform-azurerm-avm-res-app-containerapp#90
module "container_app" {
  source  = "Azure/avm-res-app-containerapp/azurerm"
  version = "0.6.0"

  name                         = "ca-api-prod"
  resource_group_name          = "rg-compute"
  container_app_environment_id = "/subscriptions/00000000/resourceGroups/rg/providers/Microsoft.App/managedEnvironments/env"

  template = {
    containers = [{
      name   = "api"
      image  = "mcr.microsoft.com/azuredocs/containerapps-helloworld:latest"
      cpu    = 0.5
      memory = "1Gi"
    }]
  }
}

# Azure Functions (Linux) - pre-azapi rewrite (v0.19.3 → v0.21.0 = full rewrite)
# Real issues: Azure/terraform-azurerm-avm-res-web-site#259, #260
module "function_linux" {
  source  = "Azure/avm-res-web-site/azurerm"
  version = "0.19.3"

  name                = "func-linux-prod"
  resource_group_name = "rg-compute"
  location            = "westeurope"

  kind = "functionapp,linux"

  os_type          = "Linux"
  service_plan_id  = "/subscriptions/00000000/resourceGroups/rg/providers/Microsoft.Web/serverFarms/plan"

  site_config = {
    application_stack = {
      python_version = "3.11"
    }
  }
}

# Azure Functions (Windows)
module "function_windows" {
  source  = "Azure/avm-res-web-site/azurerm"
  version = "0.19.3"

  name                = "func-win-prod"
  resource_group_name = "rg-compute"
  location            = "westeurope"

  kind = "functionapp"

  os_type          = "Windows"
  service_plan_id  = "/subscriptions/00000000/resourceGroups/rg/providers/Microsoft.Web/serverFarms/plan-win"

  site_config = {
    application_stack = {
      dotnet_version = "8.0"
    }
  }
}
