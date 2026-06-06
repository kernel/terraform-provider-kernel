#!/usr/bin/env bash
# Regenerates docs/ from the provider schema with tfplugindocs. Wired into
# `go generate ./...` via the directive in cmd/terraform-provider-kernel;
# CI's drift gate reuses it from scripts/check-docs.sh.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir="$(mktemp -d "${TMPDIR:-/tmp}/kernel-tfdocs.XXXXXX")"
tfplugindocs_version="${TFPLUGINDOCS_VERSION:-v0.25.0}"

cleanup() {
	rm -rf "$workdir"
}
trap cleanup EXIT

command -v terraform >/dev/null || {
	echo "terraform is required to generate provider docs" >&2
	exit 1
}

command -v jq >/dev/null || {
	echo "jq is required to normalize the provider schema for tfplugindocs" >&2
	exit 1
}

mkdir -p "$workdir/plugins" "$workdir/work"

cd "$root"
go build -o "$workdir/plugins/terraform-provider-kernel" ./cmd/terraform-provider-kernel

cat >"$workdir/work/main.tf" <<'EOF'
terraform {
  required_providers {
    kernel = {
      source = "kernel/kernel"
    }
  }
}

provider "kernel" {
  api_key = "placeholder"
}
EOF

cat >"$workdir/terraformrc" <<EOF
provider_installation {
  dev_overrides {
    "kernel/kernel" = "$workdir/plugins"
  }

  direct {}
}
EOF

(
	cd "$workdir/work"
	TMPDIR="$workdir" TF_CLI_CONFIG_FILE="$workdir/terraformrc" CHECKPOINT_DISABLE=1 \
		terraform providers schema -json >"$workdir/schema.json"
)

jq '.provider_schemas.kernel = .provider_schemas["registry.terraform.io/kernel/kernel"] | del(.provider_schemas["registry.terraform.io/kernel/kernel"])' \
	"$workdir/schema.json" >"$workdir/schema-tfplugindocs.json"

go run "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@${tfplugindocs_version}" generate \
	--provider-name kernel \
	--rendered-provider-name Kernel \
	--providers-schema "$workdir/schema-tfplugindocs.json"

go run "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@${tfplugindocs_version}" validate \
	--provider-name kernel \
	--providers-schema "$workdir/schema-tfplugindocs.json"
