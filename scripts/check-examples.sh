#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir="$(mktemp -d "${TMPDIR:-/tmp}/kernel-tfexamples.XXXXXX")"

cleanup() {
	rm -rf "$workdir"
}
trap cleanup EXIT

command -v terraform >/dev/null || {
	echo "terraform is required to validate examples" >&2
	exit 1
}

mkdir -p "$workdir/plugins"

cd "$root"
go build -o "$workdir/plugins/terraform-provider-kernel" ./cmd/terraform-provider-kernel

cat >"$workdir/terraformrc" <<EOF
provider_installation {
  dev_overrides {
    "kernel/kernel" = "$workdir/plugins"
  }

  direct {}
}
EOF

validated=0
while IFS= read -r example; do
	echo "validating $example"
	(
		cd "$example"
		TF_CLI_CONFIG_FILE="$workdir/terraformrc" CHECKPOINT_DISABLE=1 \
			terraform validate -no-color
	)
	validated=$((validated + 1))
done < <(find examples -mindepth 1 -maxdepth 1 -type d -exec test -f '{}/main.tf' ';' -print | sort)

# Guard against a silent no-op: an empty examples tree (or a failed find)
# must not let the check pass without validating anything.
if [ "$validated" -eq 0 ]; then
	echo "no example configurations found under examples/" >&2
	exit 1
fi
echo "validated $validated example configurations"
