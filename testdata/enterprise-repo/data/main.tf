# ============================================================================
# Azure SQL Database — tests provider breaking changes on azurerm upgrade
# ============================================================================

resource "azurerm_mssql_server" "main" {
  name                         = "sql-enterprise-prod"
  resource_group_name          = azurerm_resource_group.data.name
  location                     = var.location
  version                      = "12.0"
  administrator_login          = "sqladmin"
  administrator_login_password = var.sql_admin_password
  minimum_tls_version          = "1.2"

  azuread_administrator {
    login_username = "platform-admins"
    object_id      = azuread_group.platform_admins.object_id
  }

  tags = {
    environment = var.environment
  }
}

resource "azurerm_mssql_database" "app" {
  name           = "db-app-prod"
  server_id      = azurerm_mssql_server.main.id
  collation      = "SQL_Latin1_General_CP1_CI_AS"
  license_type   = "LicenseIncluded"
  max_size_gb    = 50
  sku_name       = "S2"
  zone_redundant = false

  short_term_retention_policy {
    retention_days = 7
  }

  long_term_retention_policy {
    weekly_retention  = "P4W"
    monthly_retention = "P12M"
    yearly_retention  = "P5Y"
    week_of_year      = 1
  }

  tags = {
    environment = var.environment
  }
}

resource "azurerm_mssql_database" "analytics" {
  name           = "db-analytics-prod"
  server_id      = azurerm_mssql_server.main.id
  collation      = "SQL_Latin1_General_CP1_CI_AS"
  license_type   = "LicenseIncluded"
  max_size_gb    = 100
  sku_name       = "S3"
  zone_redundant = false
}

# SQL firewall rules
resource "azurerm_mssql_firewall_rule" "allow_azure" {
  name             = "AllowAzureServices"
  server_id        = azurerm_mssql_server.main.id
  start_ip_address = "0.0.0.0"
  end_ip_address   = "0.0.0.0"
}

# SQL VNet rule
resource "azurerm_mssql_virtual_network_rule" "app_subnet" {
  name      = "sql-vnet-rule-app"
  server_id = azurerm_mssql_server.main.id
  subnet_id = module.vnet_main.subnets["data"].resource_id
}

# Diagnostic settings
resource "azurerm_monitor_diagnostic_setting" "sql" {
  name               = "diag-sql-to-log"
  target_resource_id = azurerm_mssql_database.app.id

  log_analytics_workspace_id = azurerm_log_analytics_workspace.main.id

  log {
    category = "SQLSecurityAuditEvents"
    enabled  = true

    retention_policy {
      enabled = true
      days    = 90
    }
  }

  metric {
    category = "Basic"
    enabled  = true

    retention_policy {
      enabled = true
      days    = 30
    }
  }
}

# ============================================================================
# azapi resources — tests azapi v1 → v2 breaking changes (body type changed)
# ============================================================================

# Azure SQL auditing via azapi (tests body = jsonencode(...) → body = {...} migration)
resource "azapi_resource" "sql_auditing" {
  type      = "Microsoft.Sql/servers/auditingSettings@2023-08-01-preview"
  name      = "default"
  parent_id = azurerm_mssql_server.main.id

  body = jsonencode({
    properties = {
      state                  = "Enabled"
      isAzureMonitorTargetEnabled = true
      retentionDays          = 90
    }
  })
}

# Private DNS zone via azapi
resource "azapi_resource" "private_dns_zone" {
  type      = "Microsoft.Network/privateDnsZones@2020-06-01"
  name      = "privatelink.database.windows.net"
  parent_id = azurerm_resource_group.data.id
  location  = "global"

  body = jsonencode({
    properties = {}
  })
}

# Private endpoint for SQL via azapi
resource "azapi_resource" "sql_private_endpoint" {
  type      = "Microsoft.Network/privateEndpoints@2023-09-01"
  name      = "pe-sql-prod"
  parent_id = azurerm_resource_group.data.id
  location  = var.location

  body = jsonencode({
    properties = {
      subnet = {
        id = module.vnet_main.subnets["data"].resource_id
      }
      privateLinkServiceConnections = [{
        name = "pe-sql-connection"
        properties = {
          privateLinkServiceId = azurerm_mssql_server.main.id
          groupIds             = ["sqlServer"]
        }
      }]
    }
  })

  response_export_values = [
    "properties.networkInterfaces[0].id",
    "properties.customDnsConfigs"
  ]
}
