resource "azurerm_virtual_network" "hub" {
  name                = "vnet-hub-prod"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.networking.location
  resource_group_name = azurerm_resource_group.networking.name

  tags = {
    role = "hub"
  }
}

resource "azurerm_virtual_network" "spoke_app" {
  name                = "vnet-spoke-app-prod"
  address_space       = ["10.1.0.0/16"]
  location            = azurerm_resource_group.networking.location
  resource_group_name = azurerm_resource_group.networking.name
}

resource "azurerm_virtual_network" "spoke_data" {
  name                = "vnet-spoke-data-prod"
  address_space       = ["10.2.0.0/16"]
  location            = azurerm_resource_group.networking.location
  resource_group_name = azurerm_resource_group.networking.name
}

resource "azurerm_subnet" "firewall" {
  name                 = "AzureFirewallSubnet"
  resource_group_name  = azurerm_resource_group.networking.name
  virtual_network_name = azurerm_virtual_network.hub.name
  address_prefixes     = ["10.0.0.0/24"]
}

resource "azurerm_subnet" "gateway" {
  name                 = "GatewaySubnet"
  resource_group_name  = azurerm_resource_group.networking.name
  virtual_network_name = azurerm_virtual_network.hub.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_subnet" "app" {
  name                 = "snet-app"
  resource_group_name  = azurerm_resource_group.networking.name
  virtual_network_name = azurerm_virtual_network.spoke_app.name
  address_prefixes     = ["10.1.0.0/24"]
}

resource "azurerm_subnet" "data" {
  name                 = "snet-data"
  resource_group_name  = azurerm_resource_group.networking.name
  virtual_network_name = azurerm_virtual_network.spoke_data.name
  address_prefixes     = ["10.2.0.0/24"]
}

resource "azurerm_virtual_network_peering" "hub_to_spoke_app" {
  name                      = "peer-hub-to-spoke-app"
  resource_group_name       = azurerm_resource_group.networking.name
  virtual_network_name      = azurerm_virtual_network.hub.name
  remote_virtual_network_id = azurerm_virtual_network.spoke_app.id
  allow_forwarded_traffic   = true
  allow_gateway_transit     = true
}

resource "azurerm_virtual_network_peering" "spoke_app_to_hub" {
  name                      = "peer-spoke-app-to-hub"
  resource_group_name       = azurerm_resource_group.networking.name
  virtual_network_name      = azurerm_virtual_network.spoke_app.name
  remote_virtual_network_id = azurerm_virtual_network.hub.id
  use_remote_gateways       = false
}
