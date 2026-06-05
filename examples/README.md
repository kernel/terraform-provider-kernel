# Kernel Provider Examples

These examples show durable Terraform configuration only.

They do not acquire browsers, release browsers, invoke apps, fetch logs, take screenshots, open live view, upload extensions, or force-delete active runtime state.

Set credentials with environment variables before running Terraform:

```sh
export KERNEL_API_KEY="..."
export KERNEL_PROJECT_ID="..."
```

Use local development overrides while the provider is unreleased. See the root [README.md](../README.md) for the `~/.terraformrc` setup.

## Examples

- [basic-browser-pool](basic-browser-pool) creates a minimal durable browser pool.
- [design-preview-browser-pool](design-preview-browser-pool) shows a browser pool shaped for repeated design-preview checks without modeling the browser sessions themselves.
- [lookups](lookups) shows read-only project, profile, proxy, and extension data sources.
- [project-scoped-browser-pool](project-scoped-browser-pool) places a browser pool in an explicit project, overriding the provider-level `project_id` default.
