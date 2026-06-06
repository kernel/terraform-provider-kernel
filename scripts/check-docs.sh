#!/usr/bin/env bash
# CI drift gate: regenerate docs/ with scripts/generate-docs.sh (the same
# entry point `go generate ./...` uses) and fail if the committed docs differ.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

bash "$root/scripts/generate-docs.sh"

cd "$root"
doc_status="$(git status --porcelain -- docs/index.md docs/resources docs/data-sources)"
if [ -n "$doc_status" ]; then
	echo "$doc_status"
	git diff -- docs/index.md docs/resources docs/data-sources
	exit 1
fi
