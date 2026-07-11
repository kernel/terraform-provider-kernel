terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

variable "project_name" {
  type        = string
  description = "Existing Kernel project name for exact lookup."
}

variable "browser_pool_name" {
  type        = string
  description = "Existing Kernel browser pool name for exact lookup."
}

variable "profile_name" {
  type        = string
  description = "Existing Kernel profile name for exact lookup."
}

variable "proxy_name" {
  type        = string
  description = "Existing Kernel proxy name for exact lookup."
}

variable "extension_name" {
  type        = string
  description = "Existing Kernel extension name for exact lookup."
}

variable "app_name" {
  type        = string
  description = "Existing running Kernel app name for exact lookup."
}

variable "app_version" {
  type        = string
  description = "Existing running Kernel app version for exact lookup."
}

data "kernel_project" "selected" {
  name = var.project_name
}

data "kernel_browser_pool" "selected" {
  name       = var.browser_pool_name
  project_id = data.kernel_project.selected.id
}

data "kernel_profile" "selected" {
  name       = var.profile_name
  project_id = data.kernel_project.selected.id
}

data "kernel_proxy" "selected" {
  name       = var.proxy_name
  project_id = data.kernel_project.selected.id
}

data "kernel_extension" "selected" {
  name       = var.extension_name
  project_id = data.kernel_project.selected.id
}

data "kernel_app" "selected" {
  app_name   = var.app_name
  version    = var.app_version
  project_id = data.kernel_project.selected.id
}

output "kernel_ids" {
  value = {
    project_id      = data.kernel_project.selected.id
    browser_pool_id = data.kernel_browser_pool.selected.id
    profile_id      = data.kernel_profile.selected.id
    proxy_id        = data.kernel_proxy.selected.id
    extension_id    = data.kernel_extension.selected.id
    app_id          = data.kernel_app.selected.id
    deployment_id   = data.kernel_app.selected.deployment_id
  }
}
