terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

resource "kernel_browser_pool" "basic" {
  name                 = "basic-pool"
  size                 = 1
  headless             = true
  kiosk_mode           = false
  stealth              = false
  start_url            = "https://example.com"
  timeout_seconds      = 90
  fill_rate_per_minute = 0
}

output "browser_pool_id" {
  value = kernel_browser_pool.basic.id
}
