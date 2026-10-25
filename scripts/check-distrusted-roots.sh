#!/bin/sh
# Fails if the CA bundle given as $1 still trusts a root that chains to the
# eMudhra intermediates whose private keys were compromised in October 2026.
# Matching is by SHA-256 fingerprint: Alpine's bundle is bare PEM with no
# certificate names, so grepping it for "emSign" always passes.
set -eu

bundle="${1:?usage: $0 <ca-bundle.pem>}"

# Roots Mozilla removed in NSS 3.131.1, plus the G3 TLS/code-signing roots and
# Sectigo's AAA Certificate Services, which cross-signs into the same chains.
distrusted='
86A1ECBA089C4A8D3BBE2734C612BA341D813E043CF9E8A862CD5C57A36BBE6B emSign ECC Root CA - G3
125609AA301DA0A249B97A8239CB6A34216F44DCAC9F3954B14292F2E8C8608F emSign Root CA - C1
BC4D809B15189D78DB3E1D8CF4F9726A795DA1643CA5F1358E1DDB0EDC0D7EB3 emSign ECC Root CA - C3
7DD78D5F4F13459A83DFF9ABBBA62EDBAF6F2D102BF257FD712F4D9F2746ED8D emSign Root TLS CA - G3
F2EDAAFF3D9B701FB4D14C9F285F8C0BAD90FC3BCC1FA07D47F4718EDABA34A5 emSign Root CS CA - G3
D7A7A0FB5D7E2731D771E9484EBCDEF71D5F0C3E0A2948782BC83EE0EA699EF4 AAA Certificate Services
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
	fp="$(openssl x509 -in "$cert" -noout -fingerprint -sha256 | sed 's/.*=//; s/://g')"
	match="$(printf '%s\n' "$distrusted" | grep "^$fp " || true)"
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
