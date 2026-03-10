package breaking

func (kb *KnowledgeBase) registerAzureRM() {
	// azurerm v3 → v4 breaking changes
	kb.register("azurerm", []BreakingChange{
		// Resource renames
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_app_service",
			Kind:           ResourceSplit,
			Severity:       SeverityBreaking,
			Description:    "azurerm_app_service has been removed. Use azurerm_linux_web_app or azurerm_windows_web_app instead.",
			MigrationGuide: "Replace azurerm_app_service with azurerm_linux_web_app or azurerm_windows_web_app depending on OS type.",
			OldValue:       "azurerm_app_service",
			NewValue:       "azurerm_linux_web_app / azurerm_windows_web_app",
			AutoFixable:    false,
			EffortLevel:    "large",
			BeforeSnippet: `resource "azurerm_app_service" "main" {
  name                = "my-app"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  app_service_plan_id = azurerm_app_service_plan.main.id

  site_config {
    dotnet_framework_version = "v6.0"
    always_on                = true
  }
}`,
			AfterSnippet: `resource "azurerm_linux_web_app" "main" {
  name                = "my-app"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  service_plan_id     = azurerm_service_plan.main.id

  site_config {
    application_stack {
      dotnet_version = "6.0"
    }
    always_on = true
  }
}`,
			Transform: &Transform{
				RenameResource: "azurerm_linux_web_app",
				RenameAttrs:    map[string]string{"app_service_plan_id": "service_plan_id"},
				RemoveAttrs:    []string{"dotnet_framework_version"},
			},
		},
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_app_service_plan",
			Kind:           ResourceRenamed,
			Severity:       SeverityBreaking,
			Description:    "azurerm_app_service_plan has been removed. Use azurerm_service_plan instead.",
			MigrationGuide: "Replace azurerm_app_service_plan with azurerm_service_plan.",
			OldValue:       "azurerm_app_service_plan",
			NewValue:       "azurerm_service_plan",
			AutoFixable:    false,
			EffortLevel:    "medium",
			BeforeSnippet: `resource "azurerm_app_service_plan" "main" {
  name                = "my-plan"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  sku {
    tier = "Standard"
    size = "S1"
  }
}`,
			AfterSnippet: `resource "azurerm_service_plan" "main" {
  name                = "my-plan"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  os_type             = "Linux"
  sku_name            = "S1"
}`,
			Transform: &Transform{
				RenameResource: "azurerm_service_plan",
				RemoveAttrs:    []string{"sku"},
				AddAttrs:       map[string]string{"os_type": `"Linux"`, "sku_name": `"S1"`},
			},
		},
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_app_service_slot",
			Kind:           ResourceSplit,
			Severity:       SeverityBreaking,
			Description:    "azurerm_app_service_slot has been removed. Use azurerm_linux_web_app_slot or azurerm_windows_web_app_slot.",
			OldValue:       "azurerm_app_service_slot",
			NewValue:       "azurerm_linux_web_app_slot / azurerm_windows_web_app_slot",
			AutoFixable:    false,
			EffortLevel:    "large",
			BeforeSnippet: `resource "azurerm_app_service_slot" "staging" {
  name                = "staging"
  app_service_name    = azurerm_app_service.main.name
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  app_service_plan_id = azurerm_app_service_plan.main.id
}`,
			AfterSnippet: `resource "azurerm_linux_web_app_slot" "staging" {
  name           = "staging"
  app_service_id = azurerm_linux_web_app.main.id

  site_config {}
}`,
			Transform: &Transform{
				RenameResource: "azurerm_linux_web_app_slot",
				RenameAttrs:    map[string]string{"app_service_plan_id": "service_plan_id", "app_service_name": "app_service_id"},
			},
		},

		// Attribute changes: resource_group_name → parent_id pattern
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			Attribute:      "resource_group_name",
			Kind:           BehaviorChanged,
			Severity:       SeverityWarning,
			Description:    "Several resources have moved from resource_group_name + name to using parent_id or resource_group_id.",
			MigrationGuide: "Review resources using resource_group_name and update to use the new parent_id or resource_group_id pattern where applicable.",
			AutoFixable:    false,
			EffortLevel:    "small",
			BeforeSnippet: `resource "azurerm_subnet" "example" {
  name                 = "my-subnet"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.1.0/24"]
}`,
			AfterSnippet: `# Check if your specific resource now uses parent_id.
# Many resources still accept resource_group_name.
# Example for resources that changed:
resource "azurerm_subnet" "example" {
  name             = "my-subnet"
  virtual_network_id = azurerm_virtual_network.main.id
  address_prefixes = ["10.0.1.0/24"]
}`,
		},

		// Features block changes
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			Kind:           ProviderConfigChanged,
			Severity:       SeverityBreaking,
			Description:    "The features block in the provider configuration has been restructured. Several feature flags have been removed as their behavior is now the default.",
			MigrationGuide: "Review your features {} block and remove flags that are no longer supported. The new defaults may change behavior.",
			AutoFixable:    false,
			EffortLevel:    "small",
			BeforeSnippet: `provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy = true
    }
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}`,
			AfterSnippet: `# In v4, many feature flags are now the default.
# Remove flags that no longer exist:
provider "azurerm" {
  features {}
}`,
		},

		// Removed resources
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_template_deployment",
			Kind:           ResourceRemoved,
			Severity:       SeverityBreaking,
			Description:    "azurerm_template_deployment has been removed. Use azurerm_resource_group_template_deployment.",
			OldValue:       "azurerm_template_deployment",
			NewValue:       "azurerm_resource_group_template_deployment",
			AutoFixable:    false,
			EffortLevel:    "medium",
			BeforeSnippet: `resource "azurerm_template_deployment" "main" {
  name                = "my-deployment"
  resource_group_name = azurerm_resource_group.main.name
  template_body       = file("template.json")
  deployment_mode     = "Incremental"
}`,
			AfterSnippet: `resource "azurerm_resource_group_template_deployment" "main" {
  name                = "my-deployment"
  resource_group_id   = azurerm_resource_group.main.id
  template_content    = file("template.json")
  deployment_mode     = "Incremental"
}`,
			Transform: &Transform{
				RenameResource: "azurerm_resource_group_template_deployment",
				RenameAttrs:    map[string]string{"template_body": "template_content", "resource_group_name": "resource_group_id"},
			},
		},
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_function_app",
			Kind:           ResourceSplit,
			Severity:       SeverityBreaking,
			Description:    "azurerm_function_app has been removed. Use azurerm_linux_function_app or azurerm_windows_function_app.",
			OldValue:       "azurerm_function_app",
			NewValue:       "azurerm_linux_function_app / azurerm_windows_function_app",
			AutoFixable:    false,
			EffortLevel:    "large",
			BeforeSnippet: `resource "azurerm_function_app" "main" {
  name                       = "my-func"
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  app_service_plan_id        = azurerm_app_service_plan.main.id
  storage_account_name       = azurerm_storage_account.main.name
  storage_account_access_key = azurerm_storage_account.main.primary_access_key
}`,
			AfterSnippet: `resource "azurerm_linux_function_app" "main" {
  name                = "my-func"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  service_plan_id     = azurerm_service_plan.main.id

  storage_account_name       = azurerm_storage_account.main.name
  storage_account_access_key = azurerm_storage_account.main.primary_access_key

  site_config {}
}`,
			Transform: &Transform{
				RenameResource: "azurerm_linux_function_app",
				RenameAttrs:    map[string]string{"app_service_plan_id": "service_plan_id"},
			},
		},
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_function_app_slot",
			Kind:           ResourceSplit,
			Severity:       SeverityBreaking,
			Description:    "azurerm_function_app_slot has been removed. Use azurerm_linux_function_app_slot or azurerm_windows_function_app_slot.",
			OldValue:       "azurerm_function_app_slot",
			NewValue:       "azurerm_linux_function_app_slot / azurerm_windows_function_app_slot",
			AutoFixable:    false,
			EffortLevel:    "large",
			BeforeSnippet: `resource "azurerm_function_app_slot" "staging" {
  name                       = "staging"
  function_app_name          = azurerm_function_app.main.name
  location                   = azurerm_resource_group.main.location
  resource_group_name        = azurerm_resource_group.main.name
  app_service_plan_id        = azurerm_app_service_plan.main.id
  storage_account_name       = azurerm_storage_account.main.name
  storage_account_access_key = azurerm_storage_account.main.primary_access_key
}`,
			AfterSnippet: `resource "azurerm_linux_function_app_slot" "staging" {
  name                       = "staging"
  function_app_id            = azurerm_linux_function_app.main.id
  storage_account_name       = azurerm_storage_account.main.name
  storage_account_access_key = azurerm_storage_account.main.primary_access_key

  site_config {}
}`,
			Transform: &Transform{
				RenameResource: "azurerm_linux_function_app_slot",
				RenameAttrs:    map[string]string{"app_service_plan_id": "service_plan_id", "function_app_name": "function_app_id"},
			},
		},

		// Behavioral changes
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_kubernetes_cluster",
			Attribute:      "default_node_pool.0.os_disk_type",
			Kind:           DefaultChanged,
			Severity:       SeverityWarning,
			Description:    "Default value for os_disk_type changed from Managed to Ephemeral in some configurations.",
			MigrationGuide: "Explicitly set os_disk_type if you rely on the previous default.",
			AutoFixable:    false,
			EffortLevel:    "small",
			BeforeSnippet: `resource "azurerm_kubernetes_cluster" "main" {
  # ...
  default_node_pool {
    name       = "default"
    node_count = 3
    vm_size    = "Standard_D2_v2"
    # os_disk_type was implicitly "Managed"
  }
}`,
			AfterSnippet: `resource "azurerm_kubernetes_cluster" "main" {
  # ...
  default_node_pool {
    name         = "default"
    node_count   = 3
    vm_size      = "Standard_D2_v2"
    os_disk_type = "Managed"  # ← explicitly set to keep old behavior
  }
}`,
		},
		{
			Provider:       "azurerm",
			Version:        "4.0.0",
			ResourceType:   "azurerm_storage_account",
			Attribute:      "min_tls_version",
			Kind:           DefaultChanged,
			Severity:       SeverityWarning,
			Description:    "Default min_tls_version changed from TLS1_0 to TLS1_2.",
			MigrationGuide: "Explicitly set min_tls_version if you need a value other than TLS1_2.",
			AutoFixable:    false,
			EffortLevel:    "small",
			BeforeSnippet: `resource "azurerm_storage_account" "main" {
  name                     = "mystorageaccount"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  # min_tls_version defaulted to "TLS1_0"
}`,
			AfterSnippet: `resource "azurerm_storage_account" "main" {
  name                     = "mystorageaccount"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  # Default is now TLS1_2 — this is usually fine.
  # Only add this if you need the OLD default:
  # min_tls_version = "TLS1_0"
}`,
		},
	})

	// azurerm v2 → v3 breaking changes (for repos still on v2)
	kb.register("azurerm", []BreakingChange{
		{
			Provider:       "azurerm",
			Version:        "3.0.0",
			ResourceType:   "azurerm_virtual_machine",
			Kind:           ResourceSplit,
			Severity:       SeverityBreaking,
			Description:    "azurerm_virtual_machine should be replaced with azurerm_linux_virtual_machine or azurerm_windows_virtual_machine.",
			OldValue:       "azurerm_virtual_machine",
			NewValue:       "azurerm_linux_virtual_machine / azurerm_windows_virtual_machine",
			AutoFixable:    false,
			EffortLevel:    "large",
			BeforeSnippet: `resource "azurerm_virtual_machine" "main" {
  name                  = "my-vm"
  location              = azurerm_resource_group.main.location
  resource_group_name   = azurerm_resource_group.main.name
  network_interface_ids = [azurerm_network_interface.main.id]
  vm_size               = "Standard_D2_v2"

  storage_os_disk {
    name              = "osdisk"
    caching           = "ReadWrite"
    create_option     = "FromImage"
    managed_disk_type = "Standard_LRS"
  }
}`,
			AfterSnippet: `resource "azurerm_linux_virtual_machine" "main" {
  name                = "my-vm"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  size                = "Standard_D2_v2"
  network_interface_ids = [azurerm_network_interface.main.id]
  admin_username      = "adminuser"

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "UbuntuServer"
    sku       = "18.04-LTS"
    version   = "latest"
  }
}`,
			Transform: &Transform{
				RenameResource: "azurerm_linux_virtual_machine",
				RenameAttrs:    map[string]string{"vm_size": "size"},
			},
		},
		{
			Provider:       "azurerm",
			Version:        "3.0.0",
			ResourceType:   "azurerm_virtual_machine_scale_set",
			Kind:           ResourceSplit,
			Severity:       SeverityBreaking,
			Description:    "azurerm_virtual_machine_scale_set should be replaced with OS-specific variants.",
			OldValue:       "azurerm_virtual_machine_scale_set",
			NewValue:       "azurerm_linux_virtual_machine_scale_set / azurerm_windows_virtual_machine_scale_set",
			AutoFixable:    false,
			EffortLevel:    "large",
			BeforeSnippet: `resource "azurerm_virtual_machine_scale_set" "main" {
  name                = "my-vmss"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  upgrade_policy_mode = "Manual"
  # ...complex nested blocks...
}`,
			AfterSnippet: `resource "azurerm_linux_virtual_machine_scale_set" "main" {
  name                = "my-vmss"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  sku                 = "Standard_D2_v2"
  instances           = 2
  admin_username      = "adminuser"
  # ...restructured blocks...
}`,
			Transform: &Transform{
				RenameResource: "azurerm_linux_virtual_machine_scale_set",
			},
		},
		{
			Provider:       "azurerm",
			Version:        "3.0.0",
			ResourceType:   "azurerm_container_registry",
			Attribute:      "admin_enabled",
			Kind:           DefaultChanged,
			Severity:       SeverityWarning,
			Description:    "Default for admin_enabled changed to false.",
			AutoFixable:    false,
			EffortLevel:    "small",
			BeforeSnippet: `resource "azurerm_container_registry" "main" {
  name                = "myregistry"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  sku                 = "Standard"
  # admin_enabled defaulted to true
}`,
			AfterSnippet: `resource "azurerm_container_registry" "main" {
  name                = "myregistry"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  sku                 = "Standard"
  admin_enabled       = true  # ← explicitly set to keep old behavior
}`,
		},
	})
}
