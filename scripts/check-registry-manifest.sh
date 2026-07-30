#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="$root/terraform-registry-manifest.json"

command -v jq >/dev/null || {
	echo "jq is required to validate the Terraform Registry manifest" >&2
	exit 1
}

jq -e '
	type == "object" and
	(keys == ["metadata", "version"]) and
	.version == 1 and
	(.metadata | type == "object") and
	(.metadata | keys == ["protocol_versions"]) and
	.metadata.protocol_versions == ["6.0"]
' "$manifest" >/dev/null
