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
- Run opt-in acceptance tests with real credentials before the first public release:
  - `TF_ACC=1 KERNEL_ACC=1 KERNEL_API_KEY=... KERNEL_PROJECT_ID=... go test -count=1 -timeout=30m -v ./internal/resources/browserpool -run TestAcc`
  - or run the manual `Acceptance` GitHub Actions workflow with `KERNEL_API_KEY` and `KERNEL_PROJECT_ID` repository secrets configured.
- Verify unscoped API calls send no `X-Kernel-Project-Id` header; it is sent only when a resource-level `project_id` or the provider default resolves a project.
- Confirm `terraform-registry-manifest.json` contains protocol `["6.0"]` for Terraform Plugin Framework.
- Confirm the repository license before the first public release. Do not publish a public tag until `LICENSE` exists or the release owner has explicitly documented the licensing decision.
- Confirm GitHub private vulnerability reporting or a public security contact is configured and reflected in `SECURITY.md`.
- Confirm there is no branch named like the release tag, for example `v0.1.0`.

## Registry Release Assets

Terraform Registry provider releases are GitHub Releases with semver tags prefixed by `v`, such as `v0.1.0`.

Each release must include:

- One zip archive per target OS/architecture.
- Provider binary inside each archive named `terraform-provider-kernel_v${VERSION}`.
- Archive names shaped as `terraform-provider-kernel_${VERSION}_${OS}_${ARCH}.zip`.
- `terraform-provider-kernel_${VERSION}_manifest.json`, generated from `terraform-registry-manifest.json`.
- `terraform-provider-kernel_${VERSION}_SHA256SUMS`, covering each zip and the manifest.
- `terraform-provider-kernel_${VERSION}_SHA256SUMS.sig`, a binary detached GPG signature over the checksum file.

Do not replace or mutate assets for a published version. If an asset, checksum, signature, or manifest is wrong, cut a new version.

## GoReleaser Notes

- Prefer a tag-triggered GitHub Actions release workflow once the signing key owner is decided.
- Store the ASCII-armored private signing key as `GPG_PRIVATE_KEY` and its passphrase as `PASSPHRASE`.
- Configure GoReleaser to build the provider from `./cmd/terraform-provider-kernel`.
- Configure archives so each zip contains only the provider binary with the Terraform Registry binary name.
- Configure signing for checksum artifacts. GoReleaser documents checksum signing as the usual path for archives and packages.
- Run `goreleaser release --snapshot --clean` locally before enabling real tag releases.

## Registry Setup

- Ensure the public repository name stays lowercase and matches `terraform-provider-kernel`.
- Sign in to the Terraform Registry with the GitHub account or organization that owns the namespace.
- Add the public GPG key in the Terraform Registry before publishing.
- Confirm generated docs render with the Terraform Registry documentation preview before the first release.

## Security And State Review

- Provider `api_key` remains sensitive.
- No resource or data source exposes Kernel API keys, project creation, or project mutation.
- `internal/kernelclient` exposes durable methods only; no acquire, release, flush, force-release, screenshots, logs, live view, or app invocation.
- `kernel_browser_pool` state contains durable desired configuration only.
- Browser pool read state does not include runtime counters, standby state, leased-browser state, runtime URLs, screenshots, logs, or live-view fields.
- Delete uses `force=false`; Terraform must not terminate leased browsers as cleanup.
- Import sets the canonical browser pool ID and relies on read-after-import to settle state.
- Acceptance tests create unique resources and register cleanup without force-delete behavior.
- Generated docs match schema output from `scripts/check-docs.sh`.
- Release artifacts are signed and checksummed before the GitHub release is finalized.

References:

- HashiCorp Terraform provider publishing: https://developer.hashicorp.com/terraform/registry/providers/publishing
- HashiCorp provider registry protocol: https://developer.hashicorp.com/terraform/internals/provider-registry-protocol
- GoReleaser checksum signing: https://goreleaser.com/customization/sign/
