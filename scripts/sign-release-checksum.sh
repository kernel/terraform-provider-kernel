#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${1:?usage: sign-release-checksum.sh VERSION}"

: "${EXPECTED_GPG_FINGERPRINT:?Set GPG_FINGERPRINT as a repository Actions variable before releasing.}"
: "${GPG_PRIVATE_KEY:?Set GPG_PRIVATE_KEY as a repository Actions secret before releasing.}"
: "${PASSPHRASE:?Set PASSPHRASE as a repository Actions secret before releasing.}"

expected="$(printf '%s' "$EXPECTED_GPG_FINGERPRINT" | tr '[:lower:]' '[:upper:]' | tr -d '[:space:]')"
key_details="$(
	printf '%s' "$GPG_PRIVATE_KEY" |
		gpg --batch --with-colons --import-options show-only --import 2>/dev/null
)"
fingerprints="$(
	awk -F: '
    $1 == "sec" { primary = 1; next }
    primary && $1 == "fpr" { print $10; primary = 0 }
  ' <<<"$key_details"
)"
fingerprint_count="$(printf '%s\n' "$fingerprints" | sed '/^$/d' | wc -l | tr -d ' ')"
if [ "$fingerprint_count" -ne 1 ] || [ "$fingerprints" != "$expected" ]; then
	echo "The private key must contain exactly the Registry signing key." >&2
	exit 1
fi

printf '%s' "$GPG_PRIVATE_KEY" | gpg --batch --import

checksum="$root/dist/terraform-provider-kernel_${version}_SHA256SUMS"
test -s "$checksum"
printf '%s' "$PASSPHRASE" |
	gpg --batch --pinentry-mode loopback --passphrase-fd 0 \
		--local-user "$expected" --output "${checksum}.sig" --detach-sign "$checksum"
gpg --verify "${checksum}.sig" "$checksum"
