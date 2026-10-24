#!/usr/bin/env bash
# Regression tests for asynchronous volume input and Qt control focus handling.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
runner="${QMLTESTRUNNER:-/usr/lib/qt6/bin/qmltestrunner}"
[[ -x "$runner" ]] || { echo 'UI tests need Qt 6 qmltestrunner.' >&2; exit 1; }
export QT_QPA_PLATFORM="${SONOS_TEST_PLATFORM:-offscreen}"
export QT_QPA_PLATFORMTHEME=basic
export QT_QUICK_CONTROLS_STYLE=Basic
"$runner" -import "$root/tests/ui/imports" -input "$root/tests/ui"
