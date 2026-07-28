# Selected-Surface Acceptance Matrix

This document is the live-API release gate for the provider's selected public
surface. Unit tests remain the fast default. Acceptance tests run only through
explicit local opt-in or the manual GitHub Actions workflow.

The selected surface contains two managed resources and four read-only data
sources. It does not claim coverage for future or unregistered Kernel objects.

## Gate Rules

- Set both `TF_ACC=1` and `KERNEL_ACC=1`.
- Use unique `kernel-tf-*` names for every created fixture.
- Register cleanup as soon as a canonical ID exists.
- Verify deletion through a follow-up API read.
- Keep browser-pool deletion non-forceful.
- Never acquire, release, flush, invoke, or recover runtime state.
- Keep live tests out of pull-request CI.
- Run the complete matrix against the release commit before tagging.

## Environment

```sh
export TF_ACC=1
export KERNEL_ACC=1
export KERNEL_API_KEY=...
export KERNEL_PROJECT_ID=...
export KERNEL_ALT_PROJECT_ID=... # optional second project
export KERNEL_BASE_URL=...       # optional non-production API
```

`KERNEL_PROJECT_ID` is required for the browser-pool resource and all four data
sources. The project resource is organization-scoped and does not require it.

## Matrix

| Surface | Package | Live scenario |
| --- | --- | --- |
| `kernel_project` resource | `./internal/resources/project` | Create, rename with stable ID, no-drift plan, canonical-ID import, post-import no drift, delete, and HTTP 404 verification. |
| `kernel_browser_pool` resource | `./internal/resources/browserpool` | Create, durable update with stable ID, no-drift plan, provider-default and explicit project scope, bare and project-qualified import, non-force delete, and HTTP 404 verification. |
| `kernel_project` data source | `./internal/datasources/project` | Create a unique project fixture, read it by ID and exact name, read the provider-default project, verify durable metadata and no drift, then delete and require coded `not_found`. |
| `kernel_profile` data source | `./internal/datasources/profile` | Create a durable profile fixture through the SDK, read it by ID and exact name with explicit and default project scope, verify durable metadata and no drift, then delete and require coded `not_found`. |
| `kernel_proxy` data source | `./internal/datasources/proxy` | Create a managed datacenter proxy fixture through the SDK, read it by ID and exact name with explicit and default project scope, verify durable masked metadata and no drift, then delete and require coded `not_found`. |
| `kernel_extension` data source | `./internal/datasources/extension` | Upload a durable extension fixture through the SDK, read it by ID and exact name with explicit and default project scope, verify durable metadata excludes runtime usage, verify no drift, then delete and require coded `not_found`. |

The tests exist in the repository. That does not prove they passed against a
particular release commit; the release record supplies that evidence.

## Commands

Run packages independently for fast failure isolation:

```sh
go test -count=1 -timeout=30m -v ./internal/resources/project -run TestAcc
go test -count=1 -timeout=30m -v ./internal/resources/browserpool -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/project -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/profile -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/proxy -run TestAcc
go test -count=1 -timeout=30m -v ./internal/datasources/extension -run TestAcc
```

The manual `Acceptance` workflow runs the same six packages as separate matrix
jobs with `fail-fast: false`. Live acceptance remains a manual pre-tag gate.

## Outside The Selected Surface

The release does not include a browser-pool data source or profile, proxy,
extension, deployment, app, or API-key resources. Runtime/session operations
remain outside Terraform. Unregistered surfaces are not acceptance blockers for
this selected release.

## Release Record

Record the following in the release PR or release issue:

```text
Commit:
Workflow run URL:
API environment:
Started at:
Completed at:
Package results:
Interrupted or timed-out jobs:
Leaked-resource audit completed:
```

Do not record credentials or secret values. A process-level timeout can bypass
`t.Cleanup`; follow the cleanup procedure in
[Release And Security Checklist](release.md) before rerunning or tagging.
