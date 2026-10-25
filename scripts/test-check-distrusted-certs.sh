#!/bin/sh
# Tests check-distrusted-certs.sh against its fixtures. POSIX sh, so CI can run
# it both on the runner and under busybox in the Alpine image the check gates.
set -u

dir="$(dirname "$0")"
check="$dir/check-distrusted-certs.sh"

if ! sh "$check" "$dir/testdata/isrg-root-x1.pem"; then
	echo "FAIL: check rejected ISRG Root X1" >&2
	exit 1
fi

# Exit 1 with a match for each of the 22 certificates (14 distinct keys), not
# any failure: a missing fixture or a broken openssl also exits non-zero, and a
# mistyped hash would drop its certificates.
rc=0
out="$(sh "$check" "$dir/testdata/distrusted.pem" 2>&1)" || rc=$?
echo "$out"
matches="$(printf '%s\n' "$out" | grep '^distrusted certificate still present: ' || true)"
hits="$(printf '%s\n' "$matches" | grep -c . || true)"
keys="$(printf '%s\n' "$matches" | grep . | sort -u | wc -l | tr -d ' ')"
if [ "$rc" -ne 1 ] || [ "$hits" -ne 22 ] || [ "$keys" -ne 14 ]; then
	echo "FAIL: expected exit 1 with 22 matches over 14 keys, got exit $rc, $hits matches, $keys keys" >&2
	exit 1
fi
echo "PASS: clean root accepted; 22 certificates over 14 keys rejected"
