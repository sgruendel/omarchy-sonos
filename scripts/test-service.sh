#!/usr/bin/env bash
# Exercise Service's real JSON Lines handling against an inert local backend.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
command -v quickshell >/dev/null || { echo 'Service tests need Quickshell.' >&2; exit 1; }
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
cp "$root/Service.qml" "$root/VolumeController.qml" "$root/tests/service/shell.qml" "$fixture/"
cat > "$fixture/sonos-backend" <<'BACKEND'
#!/usr/bin/env bash
printf '%s\n' '{"type":"snapshot","status":"ready","selected":"office","volume":28}'
while IFS= read -r line; do :; done
BACKEND
chmod +x "$fixture/sonos-backend"
mkdir -m 700 "$fixture/runtime"
if ! QT_QPA_PLATFORM=offscreen QT_QPA_PLATFORMTHEME=basic QT_QUICK_CONTROLS_STYLE=Basic \
  XDG_RUNTIME_DIR="$fixture/runtime" XDG_CACHE_HOME="$fixture/cache" \
  timeout 10 quickshell -p "$fixture" --no-color > "$fixture/test.log" 2>&1; then
  cat "$fixture/test.log" >&2
  exit 1
fi
if [[ $(<"$fixture/test.log") != *"SERVICE TESTS PASSED"* ]]; then
  cat "$fixture/test.log" >&2
  exit 1
fi
echo 'Service request/snapshot regression checks passed.'
