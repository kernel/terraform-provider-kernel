# Kernel Provider Examples

These examples show durable Terraform configuration only.

They do not acquire browsers, release browsers, invoke apps, fetch logs, take screenshots, open live view, download extensions, or force-delete active runtime state.

Set the API credential before running Terraform:

```sh
export KERNEL_API_KEY="..."
```

Project-scoped examples additionally require:

```sh
export KERNEL_PROJECT_ID="..."
```

Use local development overrides while the provider is unreleased. See the root [README.md](../README.md) for the `~/.terraformrc` setup.

## Examples

- [basic-browser-pool](basic-browser-pool) creates a minimal durable browser pool.
- [design-preview-browser-pool](design-preview-browser-pool) shows a browser pool shaped for repeated design-preview checks without modeling the browser sessions themselves.
- [extension](extension) uploads an immutable extension archive and tracks exact content changes with `filesha256`.
- [lookups](lookups) shows read-only app, project, profile, proxy, and extension data sources.
- [project](project) creates a durable Kernel project with an explicit unique name.
- [project-scoped-browser-pool](project-scoped-browser-pool) places a browser pool in an explicit project, overriding the provider-level `project_id` default.
