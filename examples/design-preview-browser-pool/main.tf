terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

variable "pool_name" {
  type    = string
  default = "design-preview-pool"
}

variable "profile_id" {
  type        = string
  description = "Existing Kernel profile ID for design-preview browser state."
}

variable "proxy_id" {
  type        = string
  description = "Existing Kernel proxy ID. Leave null when the pool should not use a proxy."
  default     = null
}

variable "extension_ids" {
  type        = list(string)
  description = "Existing Kernel extension IDs to load in order."
  default     = []
}

resource "kernel_browser_pool" "design_preview" {
  name          = var.pool_name
  size          = 2
  profile_id    = var.profile_id
  proxy_id      = var.proxy_id
  extension_ids = var.extension_ids

  headless             = true
  kiosk_mode           = false
  stealth              = false
  start_url            = "https://example.com"
  timeout_seconds      = 120
  fill_rate_per_minute = 50

  viewport = {
    width        = 1440
    height       = 900
    refresh_rate = 60
  }

  chrome_policy = jsonencode({
    HomepageLocation = "https://example.com"
  })
}

output "browser_pool_id" {
  value = kernel_browser_pool.design_preview.id
}
