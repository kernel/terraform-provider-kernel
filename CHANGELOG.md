# Changelog

All notable changes to the Kernel Terraform provider are recorded here.

## Unreleased

First public release candidate:

- Provider configuration for `api_key`, `base_url`, and `project_id`.
- `kernel_project` and `kernel_browser_pool` resources for durable desired state.
- Lookup-only `kernel_project`, `kernel_profile`, `kernel_proxy`, and `kernel_extension` data sources.
- Canonical-ID import for projects and bare or project-qualified import for browser pools.
- Unit tests, generated Terraform docs, examples, CI checks, and an opt-in six-package acceptance matrix.

Intentionally not included:

- Runtime browser/session operations such as acquire, release, flush, app invocation, screenshots, logs, live view, and force recovery.
- A browser-pool data source.
- Deployment, app, and API-key data sources.
- API key, profile, proxy, extension, deployment, or app resources.
- `force_destroy` browser-pool deletion.

There is no upgrade or migration path from an earlier published version because
this repository has no published provider tags. See the
[first public release guide](docs/first-release.md).
