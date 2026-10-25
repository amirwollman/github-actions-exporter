#!/bin/sh
# Warns when the GitHub Actions runner's bundled Node 24, which runs every
# JavaScript action in the job, predates the Node2Shell fix (CVE-2026-111497,
# 24.22.1, runner 2.338.1+). Exploiting it needs a malicious Brotli response
# from GitHub or Docker over TLS, so this warns rather than blocks a release
# until hosted runners roll. Run it in each job that holds a write token.
worker="$(pgrep -a -f 'bin/Runner.Worker' | awk '{print $2; exit}')"
node="$(dirname "$(dirname "$worker")")/externals/node24/bin/node"
v="$("$node" --version 2>/dev/null || echo unknown)"
echo "runner node24: $v"
if [ "$v" = unknown ] || [ "$(printf '%s\n' v24.22.1 "$v" | sort -V | head -1)" != v24.22.1 ]; then
	echo "::warning::runner's bundled Node 24 is $v, before the Node2Shell fix (24.22.1)"
fi
