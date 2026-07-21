terraform {
  required_version = ">= 1.11.0"

  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

variable "extension_zip_path" {
  type        = string
  description = "Path to a Chrome extension ZIP with a Manifest V3 manifest."
}

resource "kernel_extension" "example" {
  name          = "productivity-tools"
  source_path   = var.extension_zip_path
  source_sha256 = filesha256(var.extension_zip_path)
}
