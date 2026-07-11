# v1 Acceptance Matrix

This document is the live-API release gate for the first public v1. Unit tests
remain the fast default; acceptance tests run only through explicit local opt-in
or the manual GitHub Actions workflow.

## Gate Rules

- Set both `TF_ACC=1` and `KERNEL_ACC=1`.
- Use unique `kernel-tf-*` names for every created fixture.
- Register cleanup as soon as a canonical ID exists.
- Verify deletion with a coded `not_found` response where the API supports it.
- Never acquire, release, flush, invoke, force-release, or recover runtime state.
- Keep live tests out of pull-request CI.
- Record one complete green matrix run against the release commit before tagging.
- Every registered v1 surface must have a live acceptance test and pass against
  the release commit. A fixture blocker is a v1 tag blocker, not a release-note
  exception. An unregistered deferred resource is not part of the live matrix.

## Environment

The current workflow uses:

```sh
export TF_ACC=1
export KERNEL_ACC=1
export KERNEL_API_KEY=...
export KERNEL_PROJECT_ID=...
export KERNEL_ALT_PROJECT_ID=... # optional second project
export KERNEL_BASE_URL=...       # optional non-production API
export KERNEL_ACC_APP_NAME=...   # release-owned running app fixture
export KERNEL_ACC_APP_VERSION=... # exact fixture version
```

The app selectors are non-secret repository variables and must identify exactly
one running app version in `KERNEL_PROJECT_ID`. Secrets must remain GitHub
Actions secrets and must not be printed.

## Current Matrix

| Surface | Repository test status | Acceptance scenario | Required follow-up |
| --- | --- | --- | --- |
| `kernel_project` resource | Test present | Create, rename, no-drift plan, import, delete, and 404 verification. | None. |
| `kernel_browser_pool` resource | Test present | Create, durable update with stable ID, no-drift plan, bare and project-qualified import paths, explicit project scope, non-force delete, and 404 verification. | Keep leased-browser conflict behavior in unit tests; acceptance must not create runtime leases. |
| `kernel_extension` resource | Test present | Upload, checksum state, no-drift plan, bare and project-qualified metadata-only import paths, content replacement with new ID, old-ID disappearance, delete, and 404 verification. | None. |
| `kernel_browser_pool` data source | Test present | A uniquely created pool is read by canonical ID and byte-exact name, including normalized durable configuration, a no-drift plan, and post-destroy coded `not_found` verification. | None. |
| `kernel_project` data source | Test present | A uniquely created project is read by canonical ID and exact name; the provider default resolves the configured project; metadata, no-drift planning, and post-destroy coded `not_found` are verified. | None. |
| `kernel_extension` data source | Test present | A uniquely uploaded extension is read by canonical ID and exact name through explicit and provider-default project scope; metadata, no-drift planning, and post-destroy coded `not_found` are verified. | None. |
| `kernel_profile` data source | Test present | A uniquely created durable profile is read by canonical ID and exact name through explicit and provider-default project scope; metadata, no-drift planning, and post-cleanup coded `not_found` are verified. | None. |
| `kernel_proxy` data source | Test present | A uniquely created managed datacenter proxy is read by canonical ID and exact name through explicit and provider-default project scope; durable type/protocol metadata, no-drift planning, and post-cleanup coded `not_found` are verified without fixture credentials. | None. |
| `kernel_app` data source | Test present | A release-owned running app version is read by exact name/version through explicit and provider-default project scope; canonical deployment metadata and no-drift planning are verified without invocation. Unit coverage verifies action-name and environment-key flattening without environment values. | Keep `KERNEL_ACC_APP_NAME` and `KERNEL_ACC_APP_VERSION` pointed at exactly one running app version in the acceptance project. |
| `kernel_deployment` data source | Test present | The deployment backing the release-owned app fixture is read by canonical ID through explicit and provider-default project scope; direct GET metadata and no-drift planning are verified without logs or event streams. Unit coverage verifies that only environment variable names enter state. | Keep the release-owned app fixture running so its deployment ID remains readable. |
| `kernel_api_key` data source | Test present | A uniquely created project-scoped key is read by canonical ID and byte-exact name; masked metadata, no plaintext state, ambiguity behavior, no-drift planning, cleanup, and coded post-cleanup absence are covered. | Run with an organization-wide administrative `KERNEL_API_KEY`; project-scoped credentials cannot create or delete the fixture. |
| Profile, proxy, deployment, and API-key resources | Deferred; unregistered | No provider surfaces yet. | Enter the matrix only after their documented API/SDK/state blockers are resolved and implementation lands. |

"Test present" describes code in the repository; it does not claim a run
against the release commit. The release record below supplies that evidence.

## Current Commands

Run the eleven existing packages independently for fast failure isolation:

```sh
go test -count=1 -timeout=30m -v ./internal/resources/project -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/project -run TestAcc
go test -count=1 -timeout=30m -v ./internal/resources/browserpool -run TestAcc
go test -count=1 -timeout=30m -v ./internal/resources/extension -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/extension -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/profile -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/proxy -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/app -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/apikey -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/deployment -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/browserpool -run TestAcc
```

The manual `Acceptance` workflow runs the same packages as separate matrix jobs
with `fail-fast: false`. Add a package to that workflow in the same PR that adds
its first live test.

## Release Record

Record this information in the v1 release PR or release issue, not in this
repository with secrets:

```text
Commit:
Workflow run URL:
API environment:
Started at:
Completed at:
Package results:
Interrupted or timed-out jobs:
Leaked-resource audit completed:
Unregistered deferred surfaces:
```

An interrupted process can bypass `t.Cleanup`. Follow the ordered cleanup and
404 verification procedure in [Release And Security Checklist](release.md)
before rerunning or tagging.
