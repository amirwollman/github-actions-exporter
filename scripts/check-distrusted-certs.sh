#!/bin/sh
# Fails if any CA file given as an argument still trusts a certificate in the chains of
# the eMudhra intermediates whose private keys were compromised in October 2026.
#
# Certificates are matched on the SHA-256 of their public key (SPKI), so a
# reissued or cross-signed copy of the same key is caught too. Alpine's bundle
# is bare PEM with no certificate names, so grepping it for "emSign" would
# always pass.
set -eu

[ "$#" -gt 0 ] || {
	echo "usage: $0 <ca-file>..." >&2
	exit 2
}
bundle="$*"

# Hashes are recomputed from the certificates published at
# https://emudhra.we-have-a-problem.com/ (roots.json, impacted-intermediates.json)
# and the curl CA bundles of 2024-09-24 and 2026-09-25. The intermediates are
# the same eight keys Go 1.27.3 refuses in crypto/x509.
distrusted='
C619E4780ED8E0ABEDA24AD732567DE4383A7EA941338F6D1A7D1B49AC792FE9 emSign Root TLS CA - G3 (direct issuer)
0A6C2B2CA5B364207954171987AC9D2178561569D8205457E77CA90C32EDE282 emSign Root CS CA - G3 (direct issuer)
8D417DB2DD8BF5E3084D1E3F196D583849D81BDD4C00C70B9D39369E96B8C782 emSign ECC Root CA - G3 (cross-signer)
BD153ED7B0434F6886B17BCE8BBE84ED340C7132D702A8F4FA318F756ECBD6F3 AAA Certificate Services (direct issuer, Sectigo cross-certificates)
B7408B4D2BE0238BA37004DD34E276C6019BD2F24C9DB7D4980F5F6C359A4BCC emSign Root CA - C1 (dropped by Mozilla alongside, precaution)
EABC185C4E82D942B1A5978BA3C0181487D6B3B9974E5C49F72F6D0BD9637150 emSign ECC Root CA - C3 (dropped by Mozilla alongside, precaution)
EBED6475C2C2D80FCB199F3FE8DD6E6B3CE60B77EB15D4E42C3DEC0C6F5B5105 EM DV TLS CA - G3B
43BFFA19D6EF651451F6841E019931E96F6F2EAEFF28AB4B6F1CE4576DE2717B EM OV TLS CA - G3B
E0254C7FCDC265E2F0F8A428DABB75AB4AE039B4872D62F5EE57F0877D5A8A41 EM EV TLS CA - G3B
49DD353DC2B919A3770645117D0E60C16115E7D1BC259EEFE734B00EA434FE5F EM DV TLS CA - G3C
63ECEFEE431D49523BC1136C4E72D0206A477086CAF606CD5CF025686C109CF8 EM OV TLS CA - G3C
5C0660A2B164910D8FF589C18E14552E9E0EC237DCEFE2CB736D41A4811F81A0 EM EV TLS CA - G3C
EC4FFA1AD0AC88081314B7AAFFEF6A91F97CD57291BB5BA686C5415F5EB4F6B6 EM OV CS CA - G3B
92C063D67E8F14DB71D288E11266FFDAF69CBF115F7BAB14F3BC2F68D25E0E30 EM OV CS CA - G3C
'

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

awk -v dir="$tmp" '
	/-----BEGIN CERTIFICATE-----/ { n++; out = sprintf("%s/%05d.pem", dir, n) }
	out != "" { print > out }
	/-----END CERTIFICATE-----/ { close(out); out = "" }
' "$@"

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
		echo "distrusted certificate still present: ${match#* }" >&2
		found=1
	fi
done

if [ "$total" -eq 0 ]; then
	echo "no certificates found in $bundle" >&2
	exit 1
fi
[ "$found" -eq 0 ] || exit 1
echo "checked $total certificates in $bundle: none distrusted"
