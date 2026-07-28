#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist="$root/dist"
project="terraform-provider-kernel"
version="${1:?usage: check-release-artifacts.sh VERSION}"
platforms=(
	darwin_amd64
	darwin_arm64
	freebsd_386
	freebsd_amd64
	freebsd_arm
	freebsd_arm64
	linux_386
	linux_amd64
	linux_arm
	linux_arm64
	windows_386
	windows_amd64
	windows_arm64
)

for command in jq unzip; do
	command -v "$command" >/dev/null || {
		echo "$command is required to verify release artifacts" >&2
		exit 1
	}
done

bash "$root/scripts/check-registry-manifest.sh"

metadata_version="$(jq -er '.version | select(type == "string" and length > 0)' "$dist/metadata.json")"
if [ "$metadata_version" != "$version" ]; then
	echo "release metadata version = $metadata_version, want $version" >&2
	exit 1
fi

checksum_name="${project}_${version}_SHA256SUMS"
manifest_name="${project}_${version}_manifest.json"
expected_names="$(mktemp "${TMPDIR:-/tmp}/kernel-release-expected.XXXXXX")"
actual_names="$(mktemp "${TMPDIR:-/tmp}/kernel-release-actual.XXXXXX")"

cleanup() {
	rm -f "$expected_names" "$actual_names"
}
trap cleanup EXIT

for platform in "${platforms[@]}"; do
	os="${platform%%_*}"
	arch="${platform#*_}"
	archive_name="${project}_${version}_${os}_${arch}.zip"
	archive="$dist/$archive_name"
	binary="${project}_v${version}"
	if [ "$os" = "windows" ]; then
		binary="${binary}.exe"
	fi

	if [ ! -f "$archive" ]; then
		echo "missing release archive: $archive_name" >&2
		exit 1
	fi

	entries="$(unzip -Z1 "$archive")"
	if [ "$entries" != "$binary" ]; then
		echo "$archive_name must contain only $binary; found:" >&2
		printf '%s\n' "$entries" >&2
		exit 1
	fi

	printf '%s\n' "$archive_name" >>"$expected_names"
done

archive_count="$(find "$dist" -maxdepth 1 -type f -name "${project}_${version}_*.zip" | wc -l | tr -d ' ')"
if [ "$archive_count" -ne "${#platforms[@]}" ]; then
	echo "release archive count = $archive_count, want ${#platforms[@]}" >&2
	exit 1
fi

if [ ! -f "$dist/$manifest_name" ]; then
	echo "missing release manifest: $manifest_name" >&2
	exit 1
fi
if ! cmp -s "$root/terraform-registry-manifest.json" "$dist/$manifest_name"; then
	echo "$manifest_name does not match terraform-registry-manifest.json" >&2
	exit 1
fi
printf '%s\n' "$manifest_name" >>"$expected_names"
sort -o "$expected_names" "$expected_names"

if [ ! -f "$dist/$checksum_name" ]; then
	echo "missing checksum file: $checksum_name" >&2
	exit 1
fi

if ! awk 'NF != 2 { exit 1 } { print $2 }' "$dist/$checksum_name" >"$actual_names"; then
	echo "$checksum_name must contain one checksum and filename per line" >&2
	exit 1
fi
sort -o "$actual_names" "$actual_names"
if ! diff -u "$expected_names" "$actual_names"; then
	echo "checksum coverage does not match the release contract" >&2
	exit 1
fi

(
	cd "$dist"
	if command -v sha256sum >/dev/null; then
		sha256sum -c "$checksum_name"
	else
		shasum -a 256 -c "$checksum_name"
	fi
)

echo "verified ${#platforms[@]} provider archives and $checksum_name"
