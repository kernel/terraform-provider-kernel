# API Key Terraform State Design

## Decision

The masked `kernel_api_key` data source is supported. It uses the tagged Kernel
Go SDK's paginated query filter, then enforces byte-exact name equality in the
provider and diagnoses ambiguous names.

The `kernel_api_key` resource is deferred. Its Terraform state shape is
accepted below, but Create and Rotate must not ship until the API provides
replayable idempotency and lets the provider identify or reject rotation of the
credential authenticating the current request. The API must also expose that
credential's effective project scope so provider-default fallback can settle
predictably into Terraform state.

## First Principles

An API key is durable infrastructure, but its plaintext is not durable API
metadata. Kernel returns plaintext exactly once after Create or Rotate. Later
reads return a masked key and stable metadata.

Terraform can manage that lifecycle only when:

- retrying an uncertain Create or Rotate returns the same key and plaintext;
- refresh never replaces plaintext state with a masked value;
- import does not pretend it can recover plaintext;
- rotation is an explicit configuration change;
- the provider cannot rotate away the credential it is using unknowingly; and
- project scope follows the provider's documented precedence and settles into
  durable state.

## Current Kernel Contract

Kernel Go SDK v0.76.0, the latest tagged release at the time of this decision,
exposes Create, Get, Update, List, Delete, and Rotate. The durable API contract
has these properties:

- Create returns a new canonical ID, plaintext key, masked key, name, creator,
  timestamps, and optional project metadata.
- Get and List return masked metadata only.
- Update changes only the name.
- Delete soft-deletes a key and rejects deletion of the current key with the
  coded `cannot_delete_current_key` error.
- Rotate creates a new canonical key, returns its plaintext once, and schedules
  the old key to expire at the earlier of its existing expiration and the grace
  deadline. Rotation never extends an old key's life.
- Rotation does not soft-delete the old key. Its expired record remains in
  ordinary active listings because that status currently means non-deleted.
- Names are not unique.
- Project-scoped callers can manage keys only in their own project and cannot
  create or access organization-wide keys.
- A deleted key is hidden from ordinary Get and a repeated Delete returns
  `not_found`.

The API supports substring query today and has a dedicated exact-name List
filter pending in Go SDK v0.77.0. The data source does not require the pending
filter: it scans every query page, post-filters names byte-for-byte, and
deduplicates canonical IDs. The provider still uses only the tagged SDK and
does not add direct HTTP or a second client.

## Future Resource State

The resource should use this state model once the API blockers are resolved:

| Attribute | Terraform behavior |
| --- | --- |
| `id` | Computed canonical API-key ID. Rotate replaces it with the new key's ID. |
| `name` | Required durable name. Updated in place. Names are labels, not identity. |
| `project_id` | Optional scope override. It defaults to the provider-level `project_id`, then the authenticated key's project binding. Null after resolution creates an organization-wide key. Changing resolved scope replaces the key. |
| `days_to_expire` | Optional lifetime from 1 through 3650 days on Create. When omitted on a new key, the key does not expire. A later finite change is valid only with a simultaneous `rotation_keeper` change; because v1 uses the seven-day default grace, the changed lifetime must be 8 through 3650 days. Removing a known finite value is rejected because Rotate cannot produce a never-expiring replacement. After metadata-only import, omission means the original relative lifetime is unmanaged. |
| `rotation_keeper` | Optional opaque string. Only a change after Create triggers Rotate. Its value is provider state, not remote metadata. |
| `key` | Computed, sensitive plaintext returned by Create or Rotate. Read preserves the prior value; import leaves it null. |
| `masked_key` | Computed durable masked value from Get. It never substitutes for `key`. |
| `created_at` | Computed creation timestamp. |
| `expires_at` | Computed nullable expiration timestamp. |
| `created_by` | Computed creator metadata. |
| `project_name` | Computed nullable project metadata. |

`key` being sensitive controls display, not storage encryption. The plaintext
remains in Terraform state after Create or Rotate so downstream configuration
can use it. Users must use an encrypted remote backend with restricted state
access. Removing the value on the next Read would make dependent configuration
unstable and would not erase it from state history.

`days_to_expire` records create-time intent; Kernel reads expose the resulting
absolute `expires_at`, not the original relative input. Import therefore cannot
reconstruct that input and must not guess it from timestamp subtraction.

Project resolution is resource override, provider default, authenticated-key
binding, then organization-wide when no project applies. State stores the
resolved project ID. The current API requires a project-scoped caller to send
`project_id` but does not expose that caller's canonical scope to this provider.
Until current-key metadata closes that gap, callers must configure either the
resource or provider project explicitly.

## Lifecycle

Create sends explicit `name`, resolved `project_id`, and `days_to_expire`
values. It disables automatic mutation retries. On success it stores the
canonical ID and plaintext before any later operation can fail.

Read uses masked Get metadata. It refreshes readable fields while preserving
the prior plaintext, create-time lifetime, and rotation keeper. A coded
`not_found` response removes the resource from state. Expiration alone does not
remove the durable key record.

Update patches a changed name before Rotate so the replacement key copies the
new name. A changed `rotation_keeper` then calls Rotate and stores the returned
new ID and plaintext immediately. If `days_to_expire` changes in the same plan,
Rotate uses that new lifetime; changing the lifetime without changing the
keeper is a plan error. The old key is no longer owned by the Terraform
resource. Kernel expires it by the grace deadline, sooner when its existing
expiration is earlier, and immediately when it is already expired. Kernel
retains the non-deleted historical record; deleting the resource later deletes
only the replacement key. Rotation uses the API's default grace and otherwise
preserves the old key's original lifetime for the replacement. Custom grace
controls are deferred until a concrete Terraform workflow requires them.

## Null, Empty, And Defaults

- `name` must contain 1 through 255 UTF-8 bytes and must equal its trimmed value.
  Empty, whitespace-only, and leading/trailing-whitespace values are invalid so
  API normalization cannot create a perpetual diff.
- `project_id` may be omitted for scope resolution, but an explicit empty string
  is invalid. A resolved null value means organization-wide.
- `days_to_expire` omitted or null means no expiry for a newly created key. On
  Rotate, omission asks Kernel to preserve the old key's original lifetime. A
  changed rotation lifetime must be at least eight days so the replacement
  outlives the seven-day default grace window; this does not restrict a new
  key's initial 1-3650-day lifetime. Removing a known finite value is a plan
  error because the API treats rotation null as preserve, not never-expiring.
  Moving to no expiry requires creating a separate key and migrating consumers
  before removing the old resource.
- `rotation_keeper` may be omitted. When configured it must be non-empty. Create
  records its initial value without an extra Rotate because Create already
  issued fresh plaintext. Import leaves it null; setting it after import is an
  explicit first rotation, and every later value change rotates again.
- The API's omitted rotation grace defaults to seven days. The v1 resource does
  not expose a second grace control.
- Nullable read fields remain Terraform null. Zero times and empty strings must
  not be used as substitutes for API null values.

Delete treats `not_found` as success. The coded self-delete response is a
diagnostic and retains the resource ID so a different provider credential can
retry. Terraform must not hide that failure or remove state.

Import accepts only the canonical API-key ID. Read then imports masked metadata.
The plaintext `key`, original `days_to_expire`, and prior rotation keeper remain
unknown or unset. Import documentation must call this metadata-only import.

## Self-Use Rule

A provider instance must be authenticated by a separate administrative key
from every `kernel_api_key` resource it manages. Provider configuration cannot
depend safely on a key created by that same provider instance.

The API already prevents self-delete, but Rotate currently allows the current
key to rotate and schedules it to expire. The provider cannot derive the
authenticated key's canonical ID from its plaintext configuration or masked
metadata. Documentation alone is not a sufficient guard for an operation that
can make the next Terraform run unable to authenticate.

Before the resource ships, Kernel must expose request-context metadata that
identifies the current key and its canonical project scope, or reject
self-rotation with a stable coded error and separately expose the effective
project binding. The provider will use that signal as an internal safety check;
current-key identity does not belong in durable Terraform state.

## Idempotency Gate

Create and Rotate have no idempotency key. A connection failure after the API
commits but before Terraform receives the response loses the only plaintext
response. Create can leave an untracked credential. Rotate can additionally
schedule the old credential to expire, and a retry can create another key.

Before the resource ships, both operations must accept an idempotency key and
replay the same canonical ID and plaintext response for the same request. It is
not enough to deduplicate by name because names are intentionally non-unique.
The implementation design must prove that its request token survives a failed
apply and process restart; API support without a stable Terraform-side token is
not sufficient. Generic SDK mutation retries remain disabled outside that
replay contract.

## Masked Data Source

The data source is independent of plaintext lifecycle. It accepts exactly one
of canonical `id` or exact `name`, scans all pages for name lookup, deduplicates
by ID, and diagnoses zero or multiple non-deleted
matches. Expired-but-not-deleted keys remain visible because they are durable
records under the current API status definition. The API's name filter follows
the production database's case- and accent-insensitive collation; the provider
must post-filter returned names byte-for-byte so Terraform's exact selector has
stable semantics. Rotation retains an old row with the same name, so name
lookup becomes ambiguous after rotation and callers must use the canonical ID.
It may expose ID, name, masked key, creator, creation/expiration timestamps, and
nullable project metadata. It must never expose plaintext, deleted audit rows,
or provider-authentication identity.

## Unblocking Checklist

- Add replayable idempotency for API-key Create and Rotate.
- Add a stable current-key/project-scope signal or coded self-rotation
  rejection plus effective-scope metadata.
- Verify nullable SDK response fields against API `null` responses.
- Acceptance-test ASCII and multibyte name boundaries against the API's
  byte-count validation.
- Acceptance-test a short initial lifetime, unchanged short-lifetime rotation,
  rejection of a changed 1-7-day rotation lifetime, and rejection of a known
  finite-to-null transition.
- Acceptance-test Create, Read, rename, explicit rotation, metadata-only
  import, first rotation after import, project scope, retained rotation history,
  near-expiry and already-expired rotation, not-found deletion, and self-use
  diagnostics.
- Repeat the sensitive-state and release review before registering the resource.
