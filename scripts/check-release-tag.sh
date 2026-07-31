#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: check-release-tag.sh TAG EXPECTED_COMMIT}"
expected="${2:?usage: check-release-tag.sh TAG EXPECTED_COMMIT}"
ref="refs/tags/${tag}"

actual="$(
	git ls-remote origin "$ref" "${ref}^{}" |
		awk -v ref="$ref" '
		  $2 == ref { direct = $1 }
		  $2 == ref "^{}" { peeled = $1 }
		  END { print (peeled != "" ? peeled : direct) }
		'
)"
if [ "$actual" != "$expected" ]; then
	echo "Release tag ${tag} no longer points to the workflow commit." >&2
	exit 1
fi
