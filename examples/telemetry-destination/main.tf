terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

variable "otlp_endpoint" {
  type        = string
  description = "Base OTLP/HTTP collector endpoint, without a signal path such as /v1/logs."
}

variable "otlp_token" {
  type        = string
  sensitive   = true
  description = "Collector credential sent with each telemetry export."
}

resource "kernel_telemetry_destination" "example" {
  name        = "otlp-collector"
  endpoint    = var.otlp_endpoint
  description = "Browser session telemetry"
  headers = {
    "Authorization" = "Bearer ${var.otlp_token}"
  }
}

output "telemetry_destination_id" {
  value = kernel_telemetry_destination.example.id
}
