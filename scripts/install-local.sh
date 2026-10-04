#!/usr/bin/env bash
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
plugin_id=sgruendel.sonos
target="${XDG_CONFIG_HOME:-$HOME/.config}/omarchy/plugins/$plugin_id"
config_root="$(dirname -- "$(dirname -- "$target")")"
make -C "$root" build
mkdir -p "$(dirname "$target")"
staged="$(mktemp -d "$config_root/.sonos-stage.XXXXXX")"
trap 'rm -rf "$staged"' EXIT
"$root/scripts/stage-plugin.sh" "$staged"
omarchy plugin validate "$staged"
if [[ -e "$target" ]]; then
  mkdir -p "$config_root/plugin-backups"
  backup="$config_root/plugin-backups/$plugin_id.$(date +%s).$$"
  mv "$target" "$backup"
  echo "Previous plugin saved at $backup"
fi
if ! mv "$staged" "$target"; then
  [[ -z ${backup:-} ]] || mv "$backup" "$target"
  echo 'Could not replace plugin; restored the previous installation.' >&2
  exit 1
fi
trap - EXIT
omarchy-shell shell rescanPlugins
omarchy plugin enable "$plugin_id"
echo "Installed Sonos Paradise at $target"
