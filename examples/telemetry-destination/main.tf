terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

variable "honeycomb_api_key" {
  type        = string
  sensitive   = true
  description = "Honeycomb ingest key sent with each telemetry export."
}

resource "kernel_telemetry_destination" "honeycomb" {
  name        = "honeycomb"
  endpoint    = "https://api.honeycomb.io"
  description = "Browser session telemetry"
  headers = {
    "x-honeycomb-team" = var.honeycomb_api_key
  }
}

output "telemetry_destination_id" {
  value = kernel_telemetry_destination.honeycomb.id
}
