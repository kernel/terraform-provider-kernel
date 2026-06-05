# Terraform Provider Kernel

Terraform provider for durable Kernel infrastructure configuration.

This provider manages desired state only. Browser/session runtime operations stay in the Kernel SDK and API.

## Supported In v0

Provider configuration:

- `api_key`
- `base_url`
- `project_id`

Resource:

- `kernel_browser_pool`

Data sources:

- `kernel_project`
- `kernel_profile`
- `kernel_proxy`
- `kernel_extension`

Import:

- `kernel_browser_pool` by canonical browser pool ID

## Not Supported In v0

The provider intentionally does not manage:

- browser sessions
- acquire, release, or flush
- app invocation
- logs, screenshots, or live view
- runtime status or standby state
- force-release or recovery operations
- API key or project resources
- extension upload, download, delete, or Chrome Store operations

## Quickstart

Configure credentials with environment variables:

```sh
export KERNEL_API_KEY="..."
export KERNEL_PROJECT_ID="..."
```

Use the provider:

```hcl
terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {}

resource "kernel_browser_pool" "example" {
  name                 = "example-pool"
  size                 = 1
  headless             = true
  kiosk_mode           = false
  stealth              = false
  start_url            = "https://example.com"
  timeout_seconds      = 90
  fill_rate_per_minute = 0
}
```

Import an existing browser pool. The bare form resolves the project like create (provider default, else the API key's binding); use the project-qualified form to import from a different project:

```sh
terraform import kernel_browser_pool.example <browser-pool-id>
terraform import kernel_browser_pool.example <project-id>/<browser-pool-id>
```

## Local Development

Build the provider:

```sh
go build -o terraform-provider-kernel ./cmd/terraform-provider-kernel
```

Run unit tests:

```sh
go test ./...
```

Run vet:

```sh
go vet ./...
```

## Local Terraform Testing

For local Terraform CLI testing, build the provider binary and use Terraform development overrides. The override value must be the absolute directory containing the `terraform-provider-kernel` executable.

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "kernel/kernel" = "/absolute/path/to/terraform-provider-kernel"
  }

  direct {}
}
```

Then run Terraform from a directory containing provider configuration:

```sh
terraform plan
```

Do not run `terraform init` just to exercise this unreleased provider through `dev_overrides`; `init` can still try to resolve providers through the registry. Use `init` only when other providers or modules in the same configuration need it.

## Acceptance Tests

Acceptance tests are opt-in because they can create real Kernel resources.

Required environment:

```sh
export TF_ACC=1
export KERNEL_ACC=1
export KERNEL_API_KEY="..."
export KERNEL_PROJECT_ID="..."
```

Run the browser-pool acceptance test:

```sh
go test -v ./internal/resources/browserpool -run TestAccBrowserPoolLifecycle
```

The acceptance test creates uniquely named browser pools and uses durable `force=false` cleanup. It does not acquire browsers or perform runtime recovery.

## Architecture

See [docs/architecture.md](docs/architecture.md) for package layout, Terraform semantics, testing strategy, and release planning.
