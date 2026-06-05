terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

data "kernel_project" "prod" {
  name = "prod"
}

resource "kernel_browser_pool" "prod" {
  name = "prod-pool"
  size = 1

  # Overrides the provider-level project_id; changing it replaces the pool.
  project_id = data.kernel_project.prod.id
}

output "browser_pool_id" {
  value = kernel_browser_pool.prod.id
}

output "browser_pool_project_id" {
  value = kernel_browser_pool.prod.project_id
}
