# Changelog

All notable changes to the Kernel Terraform provider are recorded here.

## Unreleased

First public v1 release candidate:

- Provider configuration for `api_key`, `base_url`, and `project_id`.
- Durable `kernel_project`, `kernel_browser_pool`, and `kernel_extension` resources.
- Lookup-only `kernel_app`, `kernel_browser_pool`, `kernel_deployment`, `kernel_project`, `kernel_profile`, `kernel_proxy`, and `kernel_extension` data sources.
- Stable import for every registered resource, including project-qualified browser-pool and extension forms.
- Unit tests, generated Terraform docs, Terraform examples, CI checks, and an opt-in live acceptance matrix.

Intentionally deferred from the first public v1 because their durable API, SDK, or sensitive-state contracts are incomplete:

- Runtime browser/session operations such as acquire, release, flush, app invocation, screenshots, logs, live view, and force recovery.
- API key, profile, proxy, and deployment resources.
- API key data source.
- `force_destroy` browser-pool deletion.
- Terraform Plugin Framework code generation.

Internal v0 configurations can move to v1 without a provider source-address change. Follow [the v1 migration guide](docs/migration-v1.md) before importing existing durable objects.
