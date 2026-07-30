#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist="$root/dist"

for command in goreleaser jq; do
	command -v "$command" >/dev/null || {
		echo "$command is required to verify release artifacts" >&2
		exit 1
	}
done

cd "$root"
goreleaser release --snapshot --clean --skip=sign

version="$(jq -er '.version | select(type == "string" and length > 0)' "$dist/metadata.json")"
cp "$root/terraform-registry-manifest.json" "$dist/terraform-provider-kernel_${version}_manifest.json"
bash "$root/scripts/check-release-artifacts.sh" "$version"
