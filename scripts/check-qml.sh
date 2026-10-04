#!/usr/bin/env bash
# Checks syntax and imports against an installed Omarchy shell without enabling it.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
shell_root="${OMARCHY_PATH:-/usr/share/omarchy}/shell"
qml_tool="${QMLLINT:-/usr/lib/qt6/bin/qmllint}"
[[ -x "$qml_tool" && -d "$shell_root/Ui" ]] || {
  echo 'QML check needs qmllint and an installed Omarchy shell.' >&2
  exit 1
}
imports="$(mktemp -d)"
trap 'rm -rf "$imports"' EXIT
mkdir -p "$imports/qs"
ln -s "$shell_root/Ui" "$imports/qs/Ui"
ln -s "$shell_root/Commons" "$imports/qs/Commons"
"$qml_tool" -I "$imports" "$root/Service.qml" "$root/Widget.qml" "$root/ActionButton.qml"
