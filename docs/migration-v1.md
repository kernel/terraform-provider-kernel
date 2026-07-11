# Migrate Internal v0 Configurations To v1

The first public Kernel provider release is v1. Earlier v0 builds and tags were
internal release candidates, so there is no public registry upgrade path or
published v0 state migration.

## Existing Browser Pools

The provider source address remains `kernel/kernel`, and existing
`kernel_browser_pool` state remains the same resource type. Before upgrading:

1. Run `terraform plan` with the current internal provider build and save the output.
2. Upgrade to v1 in a non-production workspace first.
3. Run `terraform plan` again and review every durable field, especially project scope, profile/proxy references, ordered extensions, Chrome policy, viewport, launch settings, and warmup settings.
4. Do not accept a plan that introduces runtime counters, lease state, standby state, or browser-session operations; v1 does not model them.

Browser-pool deletion remains non-forceful. V1 does not terminate leased
browsers to make deletion succeed.

## Adopt Existing Resources

Add valid destination resource blocks before importing. The names and pool size
below are illustrative; reconcile every configured field with the remote object
before applying:

```hcl
resource "kernel_project" "example" {
  name = "existing-project-name"
}

resource "kernel_browser_pool" "example" {
  name = "existing-pool-name"
  size = 1
}

resource "kernel_extension" "example" {}
```

Then import each durable object. Choose only one import form for each
project-scoped resource:

```sh
terraform import kernel_project.example <project-id>

# Browser pool: choose the bare or project-qualified form.
terraform import kernel_browser_pool.example <browser-pool-id>
# or
terraform import kernel_browser_pool.example <project-id>/<browser-pool-id>

# Extension: choose the bare or project-qualified form.
terraform import kernel_extension.example <extension-id>
# or
terraform import kernel_extension.example <project-id>/<extension-id>
```

Use project-qualified import when the object is outside the resolved project
scope: the provider default when configured, otherwise the API key's project
binding. After import, run `terraform plan` and reconcile configuration with
the durable state returned by Kernel before applying changes.

Extension import is metadata-only: it cannot recover the original local archive
path. Managing future content replacement requires Terraform 1.11 or later plus
`source_path` and `source_sha256 = filesha256(source_path)`.

## Newly Available Lookups

V1 adds lookup-only app and browser-pool data sources alongside project,
profile, proxy, and extension lookups. Data sources do not adopt or mutate the
remote object. Exact lookup fails when no object or multiple objects match.

## Deferred Surfaces

Do not translate SDK or API operations into ad hoc Terraform resources while
waiting for deferred v1.x work. API key, profile, proxy, and deployment
resources remain unavailable until their documented durable and sensitive-state
contracts are complete. App invocation, browser sessions, logs, screenshots,
live view, health checks, and recovery operations remain outside Terraform.
