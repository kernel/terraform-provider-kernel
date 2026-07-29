# Kernel Terraform Provider Architecture

This document records the durable-only architecture, the implemented v0 baseline, and the target scope for the first public v1 of the Kernel Terraform provider.

## First Principles

Terraform should manage durable desired state. Kernel runtime operations stay in the Kernel SDK and API.

The provider must remain boring and direct:

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

- `kernel_browser_pool` imports by canonical browser pool ID, optionally qualified as `<project-id>/<pool-id>`.

## v1 Target Scope

The first public v1 should make durable Kernel configuration production-ready without turning Terraform into a runtime control plane. Core items are release-blocking unless the release notes explicitly defer them with an upstream API or SDK blocker.

Resources require stable identity, refresh, delete, import, and, where applicable, project-scoping and sensitive-state semantics. Data sources require stable identity, deterministic exact lookup, and, where applicable, masked sensitive metadata, pagination, and project scoping. Tooling experiments require deterministic regeneration and must preserve the handwritten lifecycle boundary.

Core v1 resources:

- `kernel_project` for basic project lifecycle; project limits remain separate and deferred
- `kernel_browser_pool`, preserving and hardening the v0 durable model
- `kernel_profile` for metadata lifecycle; runtime-written archive contents remain excluded
- `kernel_extension` for uploaded packages with stable content checksums
- `kernel_deployment` for deployment lifecycle, not app invocation

Core v1 data sources:

- `kernel_project`
- `kernel_browser_pool`
- `kernel_profile`
- `kernel_proxy`
- `kernel_extension`
- `kernel_deployment`
- `kernel_app`

Late or conditional v1 work:

- `kernel_proxy` resource, after write-only credential and import semantics are accepted
- masked `kernel_api_key` metadata lookup
- `kernel_api_key` resource, only after plaintext-once, retry, rotation, import, and provider self-use semantics are accepted
- project limits, only after their lifecycle is clearly separate from basic project management

Blocked candidates must remain unimplemented until the API and a tagged SDK expose the required durable contract. Provider code must not guess missing semantics, patch generated SDK code, or add a fallback HTTP client to bypass the durable client module.

Terraform schema and model code generation remains deferred. The current tool produced valid output but did not reduce code or review complexity, and broad OpenAPI-driven generation would further weaken the durable allowlist.

The evaluation evidence and reconsideration criteria are defined in [Terraform Framework Code Generation Decision](codegen.md).

## Explicit Non-Goals

These are intentionally not Terraform resources or actions:

- browser sessions
- acquire, release, or browser-pool flush
- app invocation
- `kernel_app` resources; apps remain lookup-only unless a later API exposes a separate durable app lifecycle
- invocation cleanup or status mutation
- logs, screenshots, live view, or telemetry streams
- runtime counters, standby state, lease state, or session state
- force-release, recovery, or managed runtime browser updates
- billing, organization membership, audit-log, managed-auth, or internal administration resources
- standalone secrets without a dedicated durable secrets API

`force_destroy` for browser pools is deferred beyond the initial v1 scope unless a demonstrated workflow justifies a later architecture amendment. Terraform should not terminate leased runtime work as ordinary durable-resource cleanup.

Chrome Web Store download is not an extension resource because downloading a package does not create a durable Kernel extension record.

## Package Layout

```text
cmd/terraform-provider-kernel/main.go
internal/provider/
internal/kernelclient/
internal/resources/<resource>/
internal/datasources/<data-source>/
internal/acctest/
docs/
examples/
```

Ownership:

- `cmd/terraform-provider-kernel` starts the provider plugin.
- `internal/provider` owns provider registration, provider schema, and resource/data-source wiring.
- `internal/kernelclient` owns SDK construction and durable API operations only.
- `internal/resources/*` owns one durable resource package per Kernel type, including schema, model, lifecycle, import, and tests.
- `internal/datasources/*` owns one data source package per Kernel durable lookup type.
- `internal/acctest` owns opt-in acceptance test helpers.

## Client Boundary

`internal/kernelclient` must expose only durable methods needed by Terraform resources and data sources.

It must not expose SDK runtime methods such as acquire, release, flush, force-release, session operations, logs, screenshots, or live view. This creates a compile-time guard against accidentally wiring runtime Kernel operations into Terraform.

The provider uses tagged releases of the Kernel Go SDK only. There is no fallback HTTP client.

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

`profile_save_changes` is intentionally omitted because saving browser-session changes is not part of the browser-pool durable contract.

`chrome_policy` is stored as written at the Terraform boundary (the raw JSON object string); Terraform semantic equality treats key-order- or whitespace-different but equivalent JSON as unchanged, and a malformed value is rejected by the attribute validator rather than during value conversion. It is normalized only for comparison and decoded into the SDK shape only at the final SDK call boundary.

`extension_ids` is modeled as an ordered list because Kernel persists extension `load_order`.

## Data Source Model

Each data source should be lookup-only and side-effect free.

Expected lookup shape:

- project: lookup current or named project metadata
- browser pool: lookup durable pool configuration
- profile: lookup profile by ID or supported selector
- proxy: lookup proxy by ID or supported selector
- extension: lookup extension by ID or supported selector
- deployment: lookup durable deployment metadata
- app: lookup a currently runnable app version without invoking it
- API key, if accepted: masked metadata only

Lookup semantics must be deterministic:

- zero exact matches fail
- one exact match succeeds
- multiple exact matches fail
- fuzzy matches never silently win

Data sources must not create, mutate, acquire, release, invoke, or recover Kernel runtime objects.

## Import Behavior

Every resource should import by canonical ID where the API can reconstruct durable state. Project-scoped resources may also accept a documented project-qualified form when needed to resolve a non-default project.

Read after import must flatten durable API state into Terraform state without introducing runtime fields. If the API cannot return create-only configuration or sensitive values, the resource must document metadata-only import or remain deferred. The provider returns a clear diagnostic instead of guessing.

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
- exercise import and real delete behavior for each resource

The current registered coverage and pre-tag live run are defined by the
[Selected-Surface Acceptance Matrix](acceptance.md).

## PR Slicing

PRs must be small, coherent, and shippable. No PR should rely on hidden follow-up work to keep the repo healthy.

Prefer PRs that add one durable behavior at a time, with tests that prove the new contract and preserve previously shipped behavior.

## Review Gates

Every PR loop has six sequential gates:

1. `deslop`
2. incremental self-review
3. `autoreview`
4. `dave-cheney-go-review`
5. `eblog-code-review`
6. final agreement pass

Loop:

1. Implement the PR scope.
2. Run gofmt, go test, go vet, and relevant Terraform validation.
3. Run all six review gates in order.
4. Fix every accepted and actionable finding.
5. Rerun tests.
6. Rerun the affected review gates.
7. Repeat until the final agreement pass is clean.
8. Push and open/update the PR.

Use additional specialist security, API-contract, or code-quality reviews when the PR's risk warrants them. If review gates conflict, choose the simpler and safer design unless it violates Terraform semantics.

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
- API and SDK blockers are either resolved or explicitly deferred.
- Release process, signing, licensing, and versioning are complete before the first public v1 publication.
