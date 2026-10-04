#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
target="${1:?Usage: stage-plugin.sh <target-directory>}"
mkdir -p "$target/bin"
for file in manifest.json Service.qml Widget.qml ActionButton.qml sonos-backend README.md LICENSE; do
  cp "$root/$file" "$target/$file"
done
cp "$root/bin/omarchy-sonos" "$target/bin/omarchy-sonos"
chmod +x "$target/sonos-backend" "$target/bin/omarchy-sonos"
