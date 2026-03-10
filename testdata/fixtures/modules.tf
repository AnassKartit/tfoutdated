module "vm" {
  source  = "Azure/avm-res-compute-virtualmachine/azurerm"
  version = "~> 0.6.0"

  name                = "test-vm"
  location            = "westeurope"
  resource_group_name = "rg-test"
}

module "local_mod" {
  source = "./modules/local"
}
