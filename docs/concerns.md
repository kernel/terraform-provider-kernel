# API, SDK, And Provider Concerns

This document records concerns found while building the v0 Kernel Terraform provider.

v0 remains intentionally narrow: provider configuration, `kernel_browser_pool`, lookup data sources, browser-pool import, tests, docs, CI, and release readiness. Items below are not permission to expand v0 unless they are explicitly marked required for v0 safety.

## Issue Disposition

### #24: `force_destroy` For `kernel_browser_pool`

Disposition: v1/future.

Current v0 behavior is deliberate:

- `kernel_browser_pool` delete calls the durable browser-pool delete API with `force=false`.
- Terraform returns a diagnostic when Kernel refuses deletion because browsers are leased.
- Terraform does not terminate leased browsers, release sessions, recover sessions, or perform runtime cleanup.

Revisit only as an explicit architecture amendment. If accepted later, the design needs clear semantics for drain-vs-kill behavior, timeouts, state preservation, diagnostics, docs, and acceptance coverage.

### #25: Terraform Plugin Framework Code Generation

Disposition: rejected for v1; future-only re-evaluation.

A v0.4.1 prototype generated valid extension data-source schema/model code, but the checked-in specification, generated output, and command totaled 151 lines to replace 57 direct Go lines and introduced non-idiomatic `Id`/`ProjectId` naming. V1 keeps schemas and models handwritten.

See [Terraform Framework Code Generation Decision](codegen.md) for the measured evidence and future reconsideration criteria. Any future evaluation must still keep CRUD semantics, exact lookup behavior, update patches, import, non-force delete, Chrome-policy normalization, sensitive state, and runtime/session exclusions handwritten.

Do not add `tfplugingen-framework`, generated provider Go, or a codegen drift check in v1.

## API And SDK Contract Gaps

These gaps make broad generated Terraform shape risky until the API/SDK contract more clearly separates durable infrastructure config from runtime behavior.

### Browser Pool Selector Echoes

Browser pool reads should return resolved durable IDs for profile and extension references. Name-only echoes are unsafe for Terraform state because the provider cannot infer canonical identity from a name.

v0 stance: model `profile_id` and ordered `extension_ids`; return diagnostics for API responses that cannot be flattened safely.

### Browser Pool `profile.save_changes`

Resolved upstream: browser-pool profile selectors are now id/name-only (`BrowserPoolProfile`, kernel-go-sdk v0.72.0), and the API silently ignores a raw `save_changes` on pool create/update instead of rejecting it. Single-session browser creation keeps `save_changes` unchanged.

v0 stance: omit `profile_save_changes` from Terraform; the pool contract has no such field.

### Timeout Contract

The OpenAPI schema, API validation, SDK docs, and API error text should agree on one timeout range.

v0 stance: validate against the API implementation observed during provider development.

### Viewport Contract

The OpenAPI schema lists fixed viewport bounds while the API accepts arbitrary positive dimensions.

v0 stance: validate viewport width and height as positive integers.

### Fill Rate Contract

The upper bound for `fill_rate_per_minute` can be organization-specific.

v0 stance: validate the lower bound locally and leave the effective upper bound to the API.

Future contract request: expose the effective per-organization limit or document that clients should only validate the lower bound.

### Update And Clear Semantics

Terraform needs stable omitted-vs-null-vs-empty behavior for optional durable fields.

Observed safe clears:

- `proxy_id`: empty string
- `extension_ids`: empty list
- `chrome_policy`: empty object
- `start_url`: empty string

Ambiguous or unsupported clears:

- `name`
- `profile_id`
- `viewport`

v0 stance: support only known-safe clear operations and return diagnostics rather than guessing.

Future contract request: document per-field clear semantics or expose explicit clear operations in the SDK/API.

### Durable Defaults And Static Limits

Generated validators need one source of truth for defaults and static limits such as:

- `headless`
- `stealth`
- `kiosk_mode`
- `timeout_seconds`
- `fill_rate_per_minute`
- `viewport.refresh_rate`
- `start_url`
- `chrome_policy`
- extension count

v0 stance: use Terraform `Optional + Computed` for server-defaulted durable fields and mirror only cheap, known static validation.

### Runtime Metadata Separation

The API/SDK should clearly identify which response fields are durable metadata and which can change because runtime browser/session activity occurred.

Fields to keep especially clear:

- profile `updated_at` and `last_used_at`
- proxy `status`, `last_checked`, and `ip_address`
- extension `last_used_at`
- browser-pool runtime counters and leased/session state

v0 stance: browser-pool resources must not store runtime counters, leased state, standby state, runtime URLs, screenshots, logs, or live-view fields. Data sources should stay lookup-oriented and should not perform runtime operations.

### Exact Lookup And Pagination

Terraform data sources need deterministic lookup semantics:

- zero exact matches fail;
- multiple exact matches fail;
- fuzzy matches must not silently win.

Future contract requests:

- exact name lookup endpoints or exact filter parameters;
- metadata `Get` endpoints where list scans are otherwise required;
- documented, stable pagination headers;
- SDK pagination helpers that treat a missing final next-offset as completion instead of an error when `has_more` is false.

### Response Field Validation

Terraform should not silently flatten present-but-invalid API response fields into null.

Future SDK request: expose stricter generated response token-kind metadata, or fail SDK unmarshalling when the JSON token kind does not match the response schema.

## Provider Follow-Up Concerns

These are provider-side follow-ups that remain within the v0/v1 boundary unless explicitly promoted.

- Keep `internal/kernelclient` durable-only as a compile-time guard.
- Keep provider `api_key` sensitive.
- Keep project scoping explicit: resource/data-source `project_id`, provider default `project_id`, then API-key-bound default when neither is set.
- Keep acceptance tests opt-in and project-scoped with `KERNEL_PROJECT_ID`.
- Keep provider schemas/models handwritten for v1; API/SDK contract cleanup alone does not make broad code generation worthwhile.
