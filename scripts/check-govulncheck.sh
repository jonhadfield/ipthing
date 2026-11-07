#!/usr/bin/env bash
# Run govulncheck and fail only on findings not listed in govulncheck-accepted.json.
# govulncheck has no native ignore list; these Echo v4 advisories are Fixed in: N/A
# and are mitigated by ExtractIPDirect / not using Proxy (see the accepted file).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ACCEPTED_FILE="${GOVULNCHECK_ACCEPTED:-$ROOT/govulncheck-accepted.json}"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... >"$TMP"

python3 - "$TMP" "$ACCEPTED_FILE" <<'PY'
import json, sys
from datetime import date

report_path, accepted_path = sys.argv[1], sys.argv[2]

raw = open(report_path).read()
dec = json.JSONDecoder()
i = 0
objs = []
while i < len(raw):
    while i < len(raw) and raw[i].isspace():
        i += 1
    if i >= len(raw):
        break
    obj, end = dec.raw_decode(raw, i)
    objs.append(obj)
    i = end

# Symbol-reachable findings only (match govulncheck text exit behaviour).
found = set()
for o in objs:
    f = o.get("finding")
    if not f:
        continue
    if any("function" in frame for frame in f.get("trace") or []):
        found.add(f["osv"])

accepted = json.load(open(accepted_path))
today = date.today()
allowed = set()
expired = []
for entry in accepted:
    vid = entry["id"]
    review_by = date.fromisoformat(entry["review_by"])
    if review_by < today:
        expired.append(f"{vid} (review_by {entry['review_by']})")
        continue
    allowed.add(vid)

if expired:
    print("govulncheck accepted entries expired; re-review:", file=sys.stderr)
    for line in expired:
        print(f"  - {line}", file=sys.stderr)
    sys.exit(1)

stale = sorted(allowed - found)
if stale:
    print("warning: accepted vulns no longer reported (remove from govulncheck-accepted.json):")
    for vid in stale:
        print(f"  - {vid}")

actionable = sorted(found - allowed)
if actionable:
    print("govulncheck found unreviewed vulnerabilities:", file=sys.stderr)
    for vid in actionable:
        print(f"  - {vid}", file=sys.stderr)
    print("\nRe-run with traces:", file=sys.stderr)
    sys.exit(2)

print("govulncheck: no unreviewed symbol-reachable vulnerabilities")
for vid in sorted(found & allowed):
    reason = next(e["reason"] for e in accepted if e["id"] == vid)
    print(f"  accepted {vid}: {reason}")
PY

status=$?
if [[ "$status" -eq 2 ]]; then
  go run golang.org/x/vuln/cmd/govulncheck@latest ./... || true
  exit 1
fi
exit "$status"
