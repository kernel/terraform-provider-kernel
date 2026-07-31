#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="$(mktemp -d)"
checksum="$root/dist/terraform-provider-kernel_test_SHA256SUMS"

cleanup() {
	rm -rf "$tmpdir"
	rm -f "$checksum" "${checksum}.sig"
}
trap cleanup EXIT

remote="$tmpdir/remote.git"
work="$tmpdir/work"
git init --bare "$remote" >/dev/null
git init "$work" >/dev/null
git -C "$work" config user.name test
git -C "$work" config user.email test@example.invalid
git -C "$work" commit --allow-empty -m first >/dev/null
first_commit="$(git -C "$work" rev-parse HEAD)"
git -C "$work" tag v0.0.1
git -C "$work" tag -a v0.0.2 -m annotated
git -C "$work" remote add origin "$remote"
git -C "$work" push origin --tags >/dev/null

(
	cd "$work"
	bash "$root/scripts/check-release-tag.sh" v0.0.1 "$first_commit"
	bash "$root/scripts/check-release-tag.sh" v0.0.2 "$first_commit"
	if bash "$root/scripts/check-release-tag.sh" v0.0.3 "$first_commit" >/dev/null 2>&1; then
		echo "Missing release tag unexpectedly passed." >&2
		exit 1
	fi

	git commit --allow-empty -m second >/dev/null
	git tag --force v0.0.1
	git push --force origin v0.0.1 >/dev/null
	if bash "$root/scripts/check-release-tag.sh" v0.0.1 "$first_commit" >/dev/null 2>&1; then
		echo "Moved release tag unexpectedly passed." >&2
		exit 1
	fi
)

passphrase="release-script-test"
generate_home="$tmpdir/generate-gnupg"
sign_home="$tmpdir/sign-gnupg"
mkdir -m 700 "$generate_home" "$sign_home"

GNUPGHOME="$generate_home" gpg --batch --pinentry-mode loopback \
	--passphrase "$passphrase" \
	--quick-generate-key "Release Test <release-test@example.invalid>" rsa2048 sign 0
fingerprint="$(
	GNUPGHOME="$generate_home" gpg --batch --with-colons --list-secret-keys |
		awk -F: '$1 == "fpr" { print $10; exit }'
)"
private_key="$(
	printf '%s' "$passphrase" |
		GNUPGHOME="$generate_home" gpg --batch --pinentry-mode loopback \
			--passphrase-fd 0 --armor --export-secret-keys "$fingerprint"
)"

mkdir -p "$root/dist"
printf '%s\n' "release script test" >"$checksum"
if GNUPGHOME="$sign_home" \
	EXPECTED_GPG_FINGERPRINT=0000000000000000000000000000000000000000 \
	GPG_PRIVATE_KEY="$private_key" \
	PASSPHRASE="$passphrase" \
	bash "$root/scripts/sign-release-checksum.sh" test >"$tmpdir/fingerprint-mismatch.out" 2>&1; then
	echo "Mismatched signing fingerprint unexpectedly passed." >&2
	exit 1
fi
grep -F "The private key must contain exactly the Registry signing key." \
	"$tmpdir/fingerprint-mismatch.out" >/dev/null
test ! -e "${checksum}.sig"

GNUPGHOME="$sign_home" \
	EXPECTED_GPG_FINGERPRINT="$fingerprint" \
	GPG_PRIVATE_KEY="$private_key" \
	PASSPHRASE="$passphrase" \
	bash "$root/scripts/sign-release-checksum.sh" test

GNUPGHOME="$sign_home" gpg --batch --verify "${checksum}.sig" "$checksum"
