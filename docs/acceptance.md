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
```

Future fixture-backed data-source tests may add narrowly named variables only
when the provider cannot create and clean up the fixture through a durable SDK
operation. Secrets must remain GitHub Actions secrets and must not be printed.

## Current Matrix

| Surface | Repository test status | Acceptance scenario | Required follow-up |
| --- | --- | --- | --- |
| `kernel_project` resource | Test present | Create, rename, no-drift plan, import, delete, and 404 verification. | None. |
| `kernel_browser_pool` resource | Test present | Create, durable update with stable ID, no-drift plan, bare and project-qualified import paths, explicit project scope, non-force delete, and 404 verification. | Keep leased-browser conflict behavior in unit tests; acceptance must not create runtime leases. |
| `kernel_extension` resource | Test present | Upload, checksum state, no-drift plan, metadata-only import, content replacement with new ID, old-ID disappearance, delete, and 404 verification. | Add project-qualified import to live coverage before the v1 tag. |
| `kernel_browser_pool` data source | Test present; tag blocker | A uniquely created pool is read by canonical ID and byte-exact name, including normalized durable configuration and a no-drift plan. | Add post-cleanup Get verification that requires coded `not_found`; the current cleanup sends Delete but does not prove absence. |
| `kernel_project` data source | Test missing; tag blocker | Unit and fake-client tests only. | Use a uniquely created `kernel_project` fixture and verify current/ID/name lookup plus no drift. |
| `kernel_extension` data source | Test missing; tag blocker | Unit and fake-client tests only. | Use a uniquely uploaded `kernel_extension` fixture and verify ID/name metadata lookup plus no drift. |
| `kernel_profile` data source | Test missing; tag blocker | Unit and fake-client tests only. | Add durable SDK fixture create/delete helpers, then verify ID/name lookup and cleanup. Do not model runtime-written profile contents. |
| `kernel_proxy` data source | Test missing; tag blocker | Unit and fake-client tests only. | Add a durable, non-secret-leaking proxy fixture strategy and verify ID/name lookup, masked metadata, and cleanup. |
| `kernel_app` data source | Fixture blocked; tag blocker | Unit, SDK transport, pagination, ambiguity, project-scope, and Framework state tests only. | Provide a release-owned running deployment fixture or a deterministic durable deployment setup. Verify exact app/version lookup without invocation and without exposing env values. |
| `kernel_api_key` data source | Deferred; unregistered | No provider surface yet. | Wait for a tagged SDK with exact-name filtering, then add masked ID/name lookup acceptance. |
| Profile, proxy, deployment, and API-key resources | Deferred; unregistered | No provider surfaces yet. | Enter the matrix only after their documented API/SDK/state blockers are resolved and implementation lands. |

"Test present" describes code in the repository; it does not claim a run
against the release commit. The release record below supplies that evidence.

## Current Commands

Run the four existing packages independently for fast failure isolation:

```sh
go test -count=1 -timeout=30m -v ./internal/resources/project -run TestAcc
go test -count=1 -timeout=30m -v ./internal/resources/browserpool -run TestAcc
go test -count=1 -timeout=30m -v ./internal/resources/extension -run TestAcc
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
