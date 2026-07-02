# Changelog

All notable changes to the Kernel Terraform provider are recorded here.

## Unreleased

Initial v0 release candidate:

- Provider configuration for `api_key`, `base_url`, and `project_id`.
- `kernel_browser_pool` resource for durable browser pool configuration.
- `kernel_project`, `kernel_profile`, `kernel_proxy`, and `kernel_extension` lookup data sources.
- `kernel_browser_pool` import support.
- Unit tests, generated Terraform docs, examples, CI checks, and opt-in acceptance test harness.

Intentionally unsupported in v0:

- Runtime browser/session operations such as acquire, release, flush, app invocation, screenshots, logs, live view, and force recovery.
- API key, project, profile, proxy, or extension resources.
- `force_destroy` browser-pool deletion.
- Terraform Plugin Framework code generation.
