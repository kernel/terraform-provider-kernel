# First Public Release

The first published Kernel Terraform provider version is v0.0.1. This
repository has no earlier published tags, so this release has no provider
upgrade or state migration path.

Existing Kernel API objects can still be adopted into Terraform through import.
That is object adoption, not migration from an older provider release.

## Public Surface

Provider configuration:

- `api_key`
- `base_url`
- `project_id`

Managed resources:

- `kernel_project`
- `kernel_browser_pool`

Read-only data sources:

- `kernel_project`
- `kernel_profile`
- `kernel_proxy`
- `kernel_extension`

Managed resources support durable lifecycle operations and canonical import.
ID and name selectors perform exact lookups. The project data source can also
resolve the provider-default project. Data sources never adopt or modify remote
objects.

## Adopt Existing Objects

Define the destination resource before importing. The configured values should
describe the durable object that already exists:

```hcl
resource "kernel_project" "existing" {
  name = "existing-project"
}

resource "kernel_browser_pool" "existing" {
  name = "existing-pool"
  size = 1
}
```

Import projects by canonical ID:

```sh
terraform import kernel_project.existing <project-id>
```

Import a browser pool with one of the following forms:

```sh
terraform import kernel_browser_pool.existing <browser-pool-id>
terraform import kernel_browser_pool.existing <project-id>/<browser-pool-id>
```

The bare browser-pool form resolves project scope from the provider default,
then from the API key binding. Use the project-qualified form when importing
from another project.

After import, run `terraform plan` and reconcile the resource configuration with
the durable state returned by Kernel before applying. Import itself does not
modify the remote object; a later apply can.

## Durable-State Boundary

Terraform state does not include browser sessions, leases, runtime counters,
standby state, logs, screenshots, live-view data, or app invocations.
Browser-pool deletion remains non-forceful and does not terminate active browser
work.

The release does not include a browser-pool, deployment, app, or API-key data
source. It also does not include managed profile, proxy, extension, deployment,
app, or API-key resources.

## Release Evidence

Before tagging, run every package in the
[Selected-Surface Acceptance Matrix](acceptance.md) against the release commit
and record the workflow evidence described there.
