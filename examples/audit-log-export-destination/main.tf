terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

variable "bucket" {
  type        = string
  description = "S3 bucket that receives the exported audit logs."
}

variable "region" {
  type        = string
  description = "AWS region of the bucket."
}

variable "role_arn" {
  type        = string
  description = "IAM role in your account that Kernel assumes to write to the bucket."
}

resource "kernel_audit_log_export_destination" "example" {
  bucket   = var.bucket
  region   = var.region
  prefix   = "kernel/audit-logs/"
  role_arn = var.role_arn
  # First apply paused, attach the trust policy below, and verify destination
  # access. Then change this to "active" and apply again to start delivery.
  status = "paused"
}

# Attach this trust policy to var.role_arn so Kernel can assume it.
output "trust_policy" {
  value = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = kernel_audit_log_export_destination.example.kernel_role_arn }
      Action    = "sts:AssumeRole"
      Condition = {
        StringEquals = {
          "sts:ExternalId" = kernel_audit_log_export_destination.example.external_id
        }
      }
    }]
  })
}
