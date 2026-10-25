#!/bin/sh
# Fails if the CA bundle given as $1 still trusts a root that chains to the
# eMudhra intermediates whose private keys were compromised in October 2026.
#
# Roots are matched on the SHA-256 of their public key (SPKI), so a reissued or
# cross-signed copy of the same key is caught too. Alpine's bundle is bare PEM
# with no certificate names, so grepping it for "emSign" would always pass.
set -eu

bundle="${1:?usage: $0 <ca-bundle.pem>}"

# The emSign roots dropped from the Mozilla store (curl CA bundle 2026-10-20),
# and Sectigo's AAA Certificate Services, which cross-signed into the same
# chains. AAA had already left the Mozilla store, but other bundles still carry
# it. Hashes were taken from the curl bundles of 2024-09-24 and 2026-09-25.
distrusted='
8D417DB2DD8BF5E3084D1E3F196D583849D81BDD4C00C70B9D39369E96B8C782 emSign ECC Root CA - G3
B7408B4D2BE0238BA37004DD34E276C6019BD2F24C9DB7D4980F5F6C359A4BCC emSign Root CA - C1
EABC185C4E82D942B1A5978BA3C0181487D6B3B9974E5C49F72F6D0BD9637150 emSign ECC Root CA - C3
BD153ED7B0434F6886B17BCE8BBE84ED340C7132D702A8F4FA318F756ECBD6F3 AAA Certificate Services
'

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

awk -v dir="$tmp" '
	/-----BEGIN CERTIFICATE-----/ { n++; out = sprintf("%s/%05d.pem", dir, n) }
	out != "" { print > out }
	/-----END CERTIFICATE-----/ { close(out); out = "" }
' "$bundle"

total=0
found=0
for cert in "$tmp"/*.pem; do
	[ -e "$cert" ] || continue
	total=$((total + 1))
	# Each step runs on its own so a certificate openssl cannot parse fails
	# the check instead of producing an empty hash that matches nothing.
	if ! openssl x509 -in "$cert" -noout -pubkey >"$cert.pub" ||
		! openssl pkey -pubin -in "$cert.pub" -outform DER >"$cert.der"; then
		echo "cannot parse certificate #$total in $bundle" >&2
		exit 1
	fi
	spki="$(sha256sum "$cert.der" | cut -d' ' -f1 | tr 'a-f' 'A-F')"
	case "$spki" in
	*[!0-9A-F]* | '')
		echo "bad SPKI hash for certificate #$total in $bundle" >&2
		exit 1
		;;
	esac
	[ "${#spki}" -eq 64 ] || {
		echo "bad SPKI hash for certificate #$total in $bundle" >&2
		exit 1
	}
	match="$(printf '%s\n' "$distrusted" | grep "^$spki " || true)"
	if [ -n "$match" ]; then
		echo "distrusted root still present: ${match#* }" >&2
		found=1
	fi
done

if [ "$total" -eq 0 ]; then
	echo "no certificates found in $bundle" >&2
	exit 1
fi
[ "$found" -eq 0 ] || exit 1
echo "checked $total roots in $bundle: none distrusted"
