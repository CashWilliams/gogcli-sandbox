#!/usr/bin/env sh
set -eu

if ! command -v gog >/dev/null 2>&1; then
  echo "gog not found in PATH" >&2
  exit 1
fi

if ! command -v python >/dev/null 2>&1; then
  echo "python not found; run these manually:" >&2
  echo "  gog gmail labels list --json" >&2
  echo "  gog calendar calendars --json" >&2
  exit 1
fi

echo "Gmail label IDs (use the first column in policy.allowed_read_labels / allowed_add_labels / allowed_remove_labels):"
printf "%-40s %s\n" "LABEL_ID" "LABEL_NAME"
python - <<'PY'
import json, subprocess, sys

p = subprocess.run(["gog", "gmail", "labels", "list", "--json"], capture_output=True, text=True)
if p.returncode != 0:
    sys.stderr.write(p.stderr)
    sys.exit(p.returncode)

data = json.loads(p.stdout or "{}").get("labels", [])
for item in data:
    name = item.get("name", "")
    lid = item.get("id", "")
    print(f"{lid:<40}\t{name}")
PY

echo ""
echo "Calendar IDs (use the first column in policy.allowed_calendars):"
printf "%-40s %s\n" "CALENDAR_ID" "CALENDAR_NAME"
python - <<'PY'
import json, subprocess, sys

p = subprocess.run(["gog", "calendar", "calendars", "--json"], capture_output=True, text=True)
if p.returncode != 0:
    sys.stderr.write(p.stderr)
    sys.exit(p.returncode)

data = json.loads(p.stdout or "{}").get("calendars", [])
for item in data:
    cid = item.get("id", "")
    summary = item.get("summary", "")
    print(f"{cid:<40}\t{summary}")
PY
