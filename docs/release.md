# Release And Security Checklist

Use this checklist before publishing a Kernel Terraform provider version.

## Release Preconditions

- For v0.0.1, present it as the first public release, not as an upgrade or migration from an earlier provider version.
- For v0.0.1, review the [first public release guide](first-release.md) and include its supported-surface and import guidance in the release notes.
- Work from a clean `main` checkout after the PR stack is merged.
- Run `bash scripts/check-docs.sh`.
- Run `bash scripts/check-markdown-links.sh`.
- Run `bash scripts/check-examples.sh`.
- Run `terraform fmt -check -recursive examples`.
- Run `go test -short -timeout=2m ./...`.
- Run `go vet ./...`.
- Run `bash scripts/check-registry-manifest.sh`.
- Run `goreleaser check`.
- Run `bash scripts/check-release-snapshot.sh`. It builds every supported target,
  checks each archive, and verifies checksum coverage. A snapshot skips signing
  and is not a publishable release.
- Run the complete [selected-surface acceptance matrix](acceptance.md) with real credentials against the release commit.
  - The `Acceptance` workflow runs all six packages as independent matrix jobs after changes reach `main` and supports manual dispatch. Keep live tests out of pull-request CI.
  - Process-level timeouts can bypass Go test cleanup. After an interrupted or hard-timeout run:
    1. In the Kernel dashboard or durable API, find projects, browser pools, profiles, proxies, and extensions named `kernel-tf-*` that were created during the failed workflow run.
    2. Delete leaked browser pools with `force=false`. If deletion conflicts with a lease, wait for the lease to end; do not force-release or recover the browser from Terraform cleanup.
    3. Delete other leaked project-scoped fixtures, then delete a leaked project only after its child resources are gone and the organization still has another active project.
    4. Read each canonical ID and require the expected not-found response before considering cleanup complete.
- Verify unscoped API calls send no `X-Kernel-Project-Id` header; it is sent only when a resource-level `project_id` or the provider default resolves a project.
- Confirm `terraform-registry-manifest.json` contains protocol `["6.0"]` for Terraform Plugin Framework.
- Confirm `LICENSE` contains the approved Apache License 2.0 text.
- Confirm immutable GitHub Releases are enabled for the repository. The
  publication job intentionally has no repository-administration permission to
  inspect or change this setting.
- Confirm GitHub private vulnerability reporting or a public security contact is configured and reflected in `SECURITY.md`.
- Store `GPG_PRIVATE_KEY` and `PASSPHRASE` as repository Actions secrets. Set
  the repository Actions variable `GPG_FINGERPRINT` to the fingerprint
  registered with the Terraform Registry.
- Protect `v*` tags with two active repository rulesets. The release-tag
  creation ruleset restricts creation and grants bypass to the `Write`,
  `Maintain`, and `Admin` repository roles. The immutable-release-tag ruleset
  restricts update and deletion with an empty bypass list. This lets Kernel
  engineers create releases without allowing a published tag to be moved or
  deleted.
- Before releasing, confirm every account with write-capable repository access
  is a Kernel engineer, no outside collaborator has that access, and both tag
  rulesets retain their expected rules and bypass lists. Ruleset configuration
  is an administrator-owned setup requirement, not a workflow runtime check.
- GitHub accounts with `Write`, `Maintain`, or `Admin` access can create Releases
  through the API; GitHub does not provide a separate release-publisher role.
  Treat every such account as release-authorized and keep that group limited to
  Kernel engineers. The creation ruleset remains the control that authorizes a
  release workflow run; the immutability ruleset protects the tag afterward.

## Registry Release Assets

Terraform Registry provider releases are GitHub Releases with semver tags prefixed by `v`, such as `v0.0.1`.

Each release must include:

- One zip archive per target OS/architecture.
- Provider binary inside each archive named `terraform-provider-kernel_v${VERSION}`.
- Archive names shaped as `terraform-provider-kernel_${VERSION}_${OS}_${ARCH}.zip`.
- `terraform-provider-kernel_${VERSION}_manifest.json`, generated from `terraform-registry-manifest.json`.
- `terraform-provider-kernel_${VERSION}_SHA256SUMS`, covering each zip and the manifest.
- `terraform-provider-kernel_${VERSION}_SHA256SUMS.sig`, a binary detached GPG signature over the checksum file.

Do not replace or mutate assets for a published version. If an asset, checksum, signature, or manifest is wrong, cut a new version.

## Supported Platforms

| Operating system | Architectures |
| --- | --- |
| Darwin | `amd64`, `arm64` |
| FreeBSD | `386`, `amd64`, `arm`, `arm64` |
| Linux | `386`, `amd64`, `arm`, `arm64` |
| Windows | `386`, `amd64`, `arm64` |

## GoReleaser Notes

- `.goreleaser.yml` is the source of truth for registry artifact names, target
  platforms, checksums, and manifest inclusion. The release workflow owns
  checksum signing and publication.
- Normal CI validates the GoReleaser configuration and registry manifest without
  building the complete platform matrix.
- `.github/workflows/release.yml` runs for `v*` tags. Its preparation job has
  read-only repository access and accepts only stable `vMAJOR.MINOR.PATCH`
  versions. It requires the Apache 2.0 license, public repository visibility,
  and a commit reachable from `main`, then builds and verifies the unsigned
  assets. The workflow artifact is retained for seven days. Only the
  tag-triggered publication job receives `contents: write`.
- Before creating a tag, run the workflow manually with the intended version.
  Manual runs create an unpushed tag only inside the ephemeral runner, build
  the same unsigned assets, and exercise checksum and GPG signing with
  `contents: read`. They never create a remote tag or GitHub Release.
- For a tag-triggered release, confirm the acceptance matrix passed, the
  creation ruleset still grants bypass only to the `Write`, `Maintain`, and
  `Admin` repository roles, and the update-and-deletion ruleset still has no
  bypass actors. Also confirm every account with write-capable access is a
  Kernel engineer and no outside collaborator has that access. The job
  revalidates the tag and checksums, requires the imported key to match
  `GPG_FINGERPRINT`, signs the checksum file, and publishes the GitHub Release.
- Failed-job reruns reuse the prepared artifact from the same workflow run. A
  full rerun replaces that run's artifact. If publication fails or is
  interrupted, it may leave a draft. Any existing draft stops retries until a
  Kernel engineer inspects and removes it manually. An existing published
  release always stops the workflow.
- For `v0.0.1`, GitHub includes the tagged
  [first public release guide](first-release.md) with the generated release
  notes. Later versions use generated release notes without first-release
  guidance.

## Registry Setup

- Ensure the public repository name stays lowercase and matches `terraform-provider-kernel`.
- Sign in to the Terraform Registry with the GitHub account or organization that owns the namespace.
- Add the public GPG key in the Terraform Registry before publishing.
- Confirm generated docs render with the Terraform Registry documentation
  preview before publishing each release.

## Security And State Review

- Provider `api_key` remains sensitive.
- `internal/kernelclient` exposes durable methods only; no acquire, release, flush, force-release, screenshots, logs, live view, or app invocation.
- Every resource state contains durable desired configuration only.
- Every data source is lookup-only and side-effect free.
- Project lifecycle uses organization-scoped endpoints and documents the permissions required for create, archive, and delete; if project limits are included later, their permissions receive a separate review.
- If API key management is included, reads expose masked metadata only; plaintext-once values are sensitive, import cannot recover plaintext, and rotation/self-use semantics have explicit safety review.
- Proxy credentials, deployment environment variables, source tokens, and other secret inputs are sensitive and preserve configured state when API reads return masked values.
- Browser pool read state does not include runtime counters, standby state, leased-browser state, runtime URLs, screenshots, logs, or live-view fields.
- Delete uses `force=false`; Terraform must not terminate leased browsers as cleanup.
- Each resource imports by canonical ID where the API can reconstruct durable state; metadata-only or unsupported imports are documented rather than guessed.
- Acceptance tests create unique resources and register cleanup without force-delete behavior.
- Generated docs match schema output from `scripts/check-docs.sh`.
- Release artifacts are signed and checksummed before the GitHub release is finalized.

References:

- HashiCorp Terraform provider publishing: https://developer.hashicorp.com/terraform/registry/providers/publishing
- HashiCorp provider registry protocol: https://developer.hashicorp.com/terraform/internals/provider-registry-protocol
- GitHub rulesets: https://docs.github.com/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets
