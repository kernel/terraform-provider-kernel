# Release And Security Checklist

Use this checklist before publishing a Kernel Terraform provider version.

## Release Preconditions

- The first public release ships as a complete v1. v0 tags and release artifacts stay internal to the Kernel organization; do not publish v0 to the public Terraform Registry.
- Work from a clean `main` checkout after the PR stack is merged.
- Run `bash scripts/check-docs.sh`.
- Run `bash scripts/check-markdown-links.sh`.
- Run `bash scripts/check-examples.sh`.
- Run `terraform fmt -check -recursive examples`.
- Run `go test -short -timeout=2m ./...`.
- Run `go vet ./...`.
- Run `goreleaser check`.
- Run `goreleaser release --snapshot --clean` and inspect the
  registry-shaped archives and checksum file in `dist/`. Confirm every archive
  contains only its provider binary and the checksum file includes the renamed
  manifest. A snapshot proves artifact construction only; it is not a signed or
  publishable release.
- Run the complete [v1 acceptance matrix](acceptance.md) for every registered v1 resource and data source with real credentials before the first public release.
- Configure `KERNEL_ACC_APP_NAME` and `KERNEL_ACC_APP_VERSION` repository variables to identify exactly one running app version in the acceptance project.
- Review the [v1 migration guide](migration-v1.md) and include it in the release notes.
- Use the commands and status table in `docs/acceptance.md` as the single source of truth. The manual `Acceptance` workflow runs all current packages in parallel; add each new package in the same PR as its first live test and keep live tests out of normal PR CI.
  - Process-level timeouts can bypass Go test cleanup. After an interrupted or hard-timeout run:
    1. In the Kernel dashboard or durable API, find projects, browser pools, extensions, profiles, and proxies named `kernel-tf-*` that were created during the failed workflow run.
    2. Delete leaked browser pools first with `force=false`. If deletion conflicts with a lease, wait for the lease to end; do not force-release or recover the browser from Terraform cleanup.
    3. Delete leaked extensions after removing any durable browser-pool references to them. Do not mutate pools or running browsers implicitly.
    4. Delete leaked profiles and managed datacenter proxies after removing durable references. Do not run proxy health checks as cleanup.
    5. Delete a leaked project only after its child resources are gone and the organization still has another active project.
    6. Do not delete the release-owned app fixture; it is not created by the acceptance run.
    7. Read each test-owned canonical resource ID and require a 404 before considering cleanup complete.
- Verify unscoped API calls send no `X-Kernel-Project-Id` header; it is sent only when a resource-level `project_id` or the provider default resolves a project.
- Confirm `terraform-registry-manifest.json` contains protocol `["6.0"]` for Terraform Plugin Framework.
- Confirm the repository license before the first public release. Do not publish a public tag until `LICENSE` exists or the release owner has explicitly documented the licensing decision.
- Confirm GitHub private vulnerability reporting or a public security contact is configured and reflected in `SECURITY.md`.
- Confirm there is no branch named like the release tag, for example `v1.0.0`.

## Registry Release Assets

Terraform Registry provider releases are GitHub Releases with semver tags prefixed by `v`, such as `v1.0.0`.

Each release must include:

- One zip archive per target OS/architecture.
- Provider binary inside each archive named `terraform-provider-kernel_v${VERSION}`.
- Archive names shaped as `terraform-provider-kernel_${VERSION}_${OS}_${ARCH}.zip`.
- `terraform-provider-kernel_${VERSION}_manifest.json`, generated from `terraform-registry-manifest.json`.
- `terraform-provider-kernel_${VERSION}_SHA256SUMS`, covering each zip and the manifest.
- `terraform-provider-kernel_${VERSION}_SHA256SUMS.sig`, a binary detached GPG signature over the checksum file.

Do not replace or mutate assets for a published version. If an asset, checksum, signature, or manifest is wrong, cut a new version.

## GoReleaser Notes

- `.goreleaser.yml` is the source of truth for registry artifact names, target
  platforms, checksums, and manifest inclusion.
- Normal CI runs `goreleaser check`; it does not cross-compile every target or
  publish artifacts.
- `.github/workflows/release.yml` runs for `v*` tags with read-only repository
  access. It requires a non-empty `LICENSE`, public repository visibility, and a
  tag commit reachable from `main`, then builds and validates the unsigned
  registry assets. The workflow artifact is retained for seven days for release
  inspection.
- Signing and publication remain separate manual release gates until release
  ownership and a protected publication workflow are configured.

## Registry Setup

- Ensure the public repository name stays lowercase and matches `terraform-provider-kernel`.
- Sign in to the Terraform Registry with the GitHub account or organization that owns the namespace.
- Add the public GPG key in the Terraform Registry before publishing.
- Confirm generated docs render with the Terraform Registry documentation preview before the first release.

## Security And State Review

- Provider `api_key` remains sensitive.
- `internal/kernelclient` exposes durable methods only; no acquire, release, flush, force-release, screenshots, logs, live view, or app invocation.
- Every resource state contains durable desired configuration plus only
  explicitly approved, computed inspection metadata that cannot be configured
  or drive diffs; no resource state is populated from event or log streams.
- Every data source is lookup-only and side-effect free.
- Project lifecycle uses organization-scoped endpoints and documents the permissions required for create, archive, and delete; if project limits are included later, their permissions receive a separate review.
- If API key management is included, reads expose masked metadata only; plaintext-once values are sensitive, import cannot recover plaintext, and rotation/self-use semantics have explicit safety review.
- Proxy credentials, deployment environment variables, source tokens, and
  other secret inputs are sensitive write-only values that never enter state;
  only explicit replacement keepers and readable masked metadata persist.
- Browser pool read state does not include runtime counters, standby state, leased-browser state, runtime URLs, screenshots, logs, or live-view fields.
- Delete uses `force=false`; Terraform must not terminate leased browsers as cleanup.
- Each resource imports by canonical ID where the API can reconstruct durable state; metadata-only or unsupported imports are documented rather than guessed.
- Acceptance tests create unique resources and register cleanup without force-delete behavior.
- Generated docs match schema output from `scripts/check-docs.sh`.
- Release artifacts are signed and checksummed before the GitHub release is finalized.

References:

- HashiCorp Terraform provider publishing: https://developer.hashicorp.com/terraform/registry/providers/publishing
- HashiCorp provider registry protocol: https://developer.hashicorp.com/terraform/internals/provider-registry-protocol
