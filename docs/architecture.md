# Kernel Terraform Provider Architecture

This document records the v0 architecture and phase-2 delivery plan for the Kernel Terraform provider.

## First Principles

Terraform should manage durable desired state. Kernel runtime operations stay in the Kernel SDK and API.

For v0, the provider must be boring and direct:

- Use Terraform Plugin Framework in Go.
- Build a Go plugin binary.
- Use the Kernel Go SDK through a thin provider-local wrapper.
- Expose only durable infrastructure configuration.
- Mark secrets sensitive.
- Preserve stable import behavior.
- Avoid computed runtime fields that create Terraform drift.
- Avoid generic abstractions unless they remove real complexity.

## v0 Scope

Provider configuration:

- `api_key`
- optional `base_url`
- optional `project_id`

Resource:

- `kernel_browser_pool`

Data sources:

- `kernel_project`
- `kernel_profile`
- `kernel_proxy`
- `kernel_extension`

Import:

- `kernel_browser_pool` imports by canonical browser pool ID.

## Explicit Non-Goals

These are intentionally not Terraform resources or actions in v0:

- browser sessions
- acquire/release
- app invocation
- logs, screenshots, or live view
- runtime status or standby state
- force-release/recovery operations
- API key resources
- project resources

## Package Layout

```text
cmd/terraform-provider-kernel/main.go
internal/provider/
internal/kernelclient/
internal/resources/browserpool/
internal/datasources/project/
internal/datasources/profile/
internal/datasources/proxy/
internal/datasources/extension/
internal/acctest/
docs/
examples/
```

Ownership:

- `cmd/terraform-provider-kernel` starts the provider plugin.
- `internal/provider` owns provider registration, provider schema, and resource/data-source wiring.
- `internal/kernelclient` owns SDK construction and durable API operations only.
- `internal/resources/browserpool` owns the browser pool Terraform schema, model, CRUD, import, and tests.
- `internal/datasources/*` owns one data source package per Kernel durable lookup type.
- `internal/acctest` owns opt-in acceptance test helpers.

## Client Boundary

`internal/kernelclient` must expose only durable methods needed by Terraform resources and data sources.

It must not expose SDK runtime methods such as acquire, release, flush, force-release, session operations, logs, screenshots, or live view. This creates a compile-time guard against accidentally wiring runtime Kernel operations into Terraform.

The provider uses the Kernel Go SDK only. There is no fallback HTTP client in v0.

## Provider Configuration

Provider config sources:

- Terraform config
- environment variables for secrets and common defaults

Rules:

- `api_key` is sensitive.
- `base_url` is optional and intended for local or non-default Kernel API targets.
- `project_id` is optional and may be overridden by resource or data source attributes where supported.
- config validation should fail early when required auth is missing.

## Browser Pool Resource Model

`kernel_browser_pool` manages durable desired configuration only.

Durable fields include:

- `id` computed canonical ID
- `name`
- `size`
- `profile_id`
- `proxy_id`
- ordered `extension_ids`
- `chrome_policy`
- `viewport`
- `headless`
- `kiosk_mode`
- `stealth`
- `start_url`
- `timeout_seconds`
- `fill_rate_per_minute`

Runtime fields are intentionally excluded, including acquired counts, available counts, standby state, lease state, and session state.

Durable fields with server defaults use Terraform `Optional + Computed` semantics so create/read/import can round-trip API-defaulted durable configuration without future preserve-null special cases. Runtime fields are still excluded rather than modeled as computed attributes.

`profile_save_changes` is intentionally omitted in v0 because the browser pool API currently rejects it for browser pools.

`chrome_policy` is stored as written at the Terraform boundary (the raw JSON object string); Terraform semantic equality treats key-order- or whitespace-different but equivalent JSON as unchanged, and a malformed value is rejected by the attribute validator rather than during value conversion. It is normalized only for comparison and decoded into the SDK shape only at the final SDK call boundary.

`extension_ids` is modeled as an ordered list because Kernel persists extension `load_order`.

## Data Source Model

Each data source should be lookup-only and side-effect free.

Expected lookup shape:

- project: lookup current or named project metadata
- profile: lookup profile by ID or supported selector
- proxy: lookup proxy by ID or supported selector
- extension: lookup extension by ID or supported selector

Data sources must not create, mutate, acquire, release, invoke, or recover Kernel runtime objects.

## Import Behavior

`kernel_browser_pool` import uses the canonical browser pool ID.

Read after import must flatten durable API state into Terraform state without introducing runtime fields. If the API returns values v0 cannot represent safely, the provider should return a clear diagnostic instead of guessing.

## Testing Strategy

Every implementation PR must include focused tests for the behavior it adds.

Test types:

- unit tests for schema flags and validators
- unit tests for expand/flatten helpers
- unit tests for provider config and client construction
- resource tests with fake or mocked durable client behavior where practical
- opt-in acceptance tests gated by explicit environment variables

Acceptance tests must:

- be disabled by default
- create uniquely named resources
- clean up after themselves
- avoid browser/session runtime operations

## PR Slicing

PRs must be small, coherent, and shippable. No PR should rely on hidden follow-up work to keep the repo healthy.

Updated PR4 split:

1. Browser pool schema/model/validators/JSON normalization.
2. Browser pool expand helpers from Terraform model to SDK params.
3. Browser pool flatten helpers from SDK response to Terraform state.

Then continue:

4. Browser pool create/read.
5. Browser pool update/delete.
6. Browser pool import.
7. Data sources.
8. Extra unit coverage and cleanup.
9. Acceptance test harness.
10. Examples/docs.
11. CI/release/security checklist.

## Review Gates

Every PR loop has five gates:

1. `deslop`
2. `autoreview`
3. `thermo-nuclear-code-quality-review`
4. `dave-cheney-go-review`
5. `eblog-code-review`

Loop:

1. Implement the PR scope.
2. Run gofmt, go test, go vet, and relevant Terraform validation.
3. Run all five review gates.
4. Fix every accepted and actionable finding.
5. Rerun tests.
6. Rerun all five review gates.
7. Repeat until all five gates are clean.
8. Push and open/update the PR.

If review gates conflict, choose the simpler and safer design unless it violates Terraform semantics.

## Release And Docs Strategy

Docs and examples should be generated or maintained with the Terraform provider schema as the source of truth where practical.

Release checklist:

- Go tests pass.
- Acceptance tests documented and opt-in.
- Provider binary builds.
- Examples validate.
- Sensitive values are marked sensitive.
- Runtime operations are absent from Terraform resources.
- Import behavior is documented.
- Release process and versioning are documented before v0 publication.
