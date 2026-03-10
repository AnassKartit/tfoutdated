# Azure Verified Module for VMs
module "linux_vm" {
  source  = "Azure/avm-res-compute-virtualmachine/azurerm"
  version = "~> 0.6.0"

  name                = "vm-app-prod-001"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  zone = 1
}

module "windows_vm" {
  source  = "Azure/avm-res-compute-virtualmachine/azurerm"
  version = "~> 0.6.0"

  name                = "vm-jump-prod-001"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  zone = 2
}

resource "azurerm_kubernetes_cluster" "main" {
  name                = "aks-enterprise-prod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  dns_prefix          = "aks-enterprise"
  kubernetes_version  = "1.28"

  default_node_pool {
    name                = "system"
    node_count          = 3
    vm_size             = "Standard_D4s_v3"
    vnet_subnet_id      = azurerm_subnet.app.id
    enable_auto_scaling = true
    min_count           = 3
    max_count           = 10
  }

  identity {
    type = "UserAssigned"
    identity_ids = [
      azurerm_user_assigned_identity.aks.id,
    ]
  }

  network_profile {
    network_plugin    = "azure"
    network_policy    = "calico"
    load_balancer_sku = "standard"
    service_cidr      = "10.100.0.0/16"
    dns_service_ip    = "10.100.0.10"
  }

  oms_agent {
    log_analytics_workspace_id = azurerm_log_analytics_workspace.main.id
  }

  tags = {
    environment = "production"
  }
}

# Legacy app service (will break on v4 upgrade)
resource "azurerm_app_service_plan" "legacy_api" {
  name                = "asp-legacy-api-prod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  sku {
    tier = "Standard"
    size = "S2"
  }
}

resource "azurerm_app_service" "legacy_api" {
  name                = "app-legacy-api-prod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  app_service_plan_id = azurerm_app_service_plan.legacy_api.id

  site_config {
    dotnet_framework_version = "v6.0"
    always_on                = true
    min_tls_version          = "1.2"
  }

  app_settings = {
    "WEBSITE_RUN_FROM_PACKAGE" = "1"
  }
}

# Legacy function app (will break on v4 upgrade)
resource "azurerm_function_app" "processor" {
  name                       = "func-processor-prod"
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  app_service_plan_id        = azurerm_app_service_plan.legacy_api.id
  storage_account_name       = azurerm_storage_account.functions.name
  storage_account_access_key = azurerm_storage_account.functions.primary_access_key
  os_type                    = "linux"
  version                    = "~4"
}

resource "azurerm_log_analytics_workspace" "main" {
  name                = "log-enterprise-prod"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "PerGB2018"
  retention_in_days   = 90
}
