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
  description = "Unique name for the Kernel project managed by Terraform."
}

resource "kernel_project" "example" {
  name = var.project_name
}

output "project_id" {
  value = kernel_project.example.id
}
