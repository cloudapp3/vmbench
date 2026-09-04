#!/usr/bin/env bash
# govulncheck gate: fail on any reachable (symbol-level) vulnerability that is
# not explicitly allowlisted below. Findings limited to imported/required
# modules without a called symbol never fail the build.
set -euo pipefail

rc=0
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 -format json ./... > /tmp/vmbench-vulncheck.json || rc=$?
if [ "$rc" -ne 0 ] && [ "$rc" -ne 3 ]; then
  echo "govulncheck could not run (exit $rc)" >&2
  exit "$rc"
fi

# GO-2026-4479 (pion/dtls AES-GCM nonce reuse, CVE-2026-26014): reachable only
# through gostun's package init chain; vmbench never establishes DTLS sessions,
# and pion/dtls/v2 has no fixed release (the fix exists only in pion/dtls v3).
# Remove this entry when gostun migrates to pion/dtls v3.
allowlist=(
  "GO-2026-4479"
)

blocked=$(jq -r 'select(.finding) | select(any(.finding.trace[]; .function != null)) | .finding.osv' /tmp/vmbench-vulncheck.json \
  | sort -u | grep -Fvx -f <(printf '%s\n' "${allowlist[@]}") || true)

if [ -n "$blocked" ]; then
  echo "Reachable vulnerabilities not allowlisted:" >&2
  echo "$blocked" >&2
  exit 1
fi
echo "govulncheck: no reachable vulnerabilities beyond the allowlist (${allowlist[*]})."
