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

data "kernel_project" "selected" {
  name = var.project_name
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

output "kernel_ids" {
  value = {
    project_id   = data.kernel_project.selected.id
    profile_id   = data.kernel_profile.selected.id
    proxy_id     = data.kernel_proxy.selected.id
    extension_id = data.kernel_extension.selected.id
  }
}
