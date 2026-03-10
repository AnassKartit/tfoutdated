#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Building tfoutdated..."
cd "$PROJECT_DIR"
go build -o /usr/local/bin/tfoutdated .

echo "==> Preparing AWS demo..."
mkdir -p /tmp/tfoutdated-demo
cat > /tmp/tfoutdated-demo/main.tf << 'EOF'
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.30"
    }
  }
}

provider "aws" {
  region = "us-east-1"
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"

  cluster_name                   = "production-cluster"
  cluster_version                = "1.27"
  cluster_endpoint_public_access = true

  vpc_id     = "vpc-12345"
  subnet_ids = ["subnet-1", "subnet-2"]

  cluster_addons = {
    coredns    = { most_recent = true }
    kube-proxy = { most_recent = true }
  }

  tags = { Environment = "production" }
}

module "s3_bucket" {
  source  = "terraform-aws-modules/s3-bucket/aws"
  version = "3.0.0"

  bucket = "my-production-bucket"
  acl    = "private"
  tags   = { Environment = "production" }
}
EOF
cp -f /tmp/tfoutdated-demo/main.tf /tmp/tfoutdated-demo/main.tf.bak

echo "==> Preparing Azure demo..."
mkdir -p /tmp/tfoutdated-demo-azure
cat > /tmp/tfoutdated-demo-azure/main.tf << 'EOF'
terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.75"
    }
  }
}

provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "main" {
  name     = "rg-production"
  location = "West Europe"
}

module "vnet" {
  source  = "Azure/avm-res-network-virtualnetwork/azurerm"
  version = "0.7.0"

  name                = "vnet-production"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  address_space       = ["10.0.0.0/16"]

  subnets = {
    default = {
      name             = "default"
      address_prefixes = ["10.0.1.0/24"]
    }
  }
}

module "acr" {
  source  = "Azure/avm-res-containerregistry-registry/azurerm"
  version = "0.4.0"

  name                = "acrproduction"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  sku                 = "Premium"
}
EOF
cp -f /tmp/tfoutdated-demo-azure/main.tf /tmp/tfoutdated-demo-azure/main.tf.bak

echo "==> Pre-warming registry cache (this takes a minute)..."
cd /tmp/tfoutdated-demo && tfoutdated scan > /dev/null 2>&1 || true
cp -f main.tf.bak main.tf
cd /tmp/tfoutdated-demo-azure && tfoutdated scan > /dev/null 2>&1 || true
cp -f main.tf.bak main.tf

echo "==> Recording demo..."
cd "$PROJECT_DIR"
vhs demo/demo.tape

echo ""
echo "✓ Done! Output: demo/demo.gif"
echo ""
echo "Add to README:"
echo '  ![tfoutdated demo](demo/demo.gif)'
