#!/bin/sh
# svcman installer — Author: Konstantinos Patronas <kpatronas@gmail.com>
#
#   sh install.sh               build and install to ~/.local/bin (no sudo)
#   sh install.sh /usr/local/bin  install elsewhere (uses sudo if the directory is not writable)
#   sh install.sh --uninstall [DIR]
set -eu
cd "$(dirname "$0")"

uninstall=
if [ "${1:-}" = --uninstall ]; then uninstall=1; shift; fi
dir=${1:-$HOME/.local/bin}
target=$dir/svcman

sudo_if_needed() {
  if [ -w "$dir" ] || { [ ! -e "$dir" ] && mkdir -p "$dir" 2>/dev/null; }; then "$@"; else sudo "$@"; fi
}

if [ -n "$uninstall" ]; then
  [ -e "$target" ] || { echo "svcman is not installed in $dir"; exit 0; }
  sudo_if_needed rm -f "$target"
  echo "removed $target"
  exit 0
fi

command -v go >/dev/null 2>&1 || { echo "install.sh: Go 1.26+ is needed to build svcman (https://go.dev/dl/)" >&2; exit 1; }
version=$(sed -n 's/^VERSION *?= *//p' Makefile)
echo "building svcman ${version}…"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$tmp" .

sudo_if_needed mkdir -p "$dir"
sudo_if_needed install -m 0755 "$tmp" "$target"
echo "installed $target"
"$target" --version | head -1

case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "note: $dir is not in your PATH; add it, e.g.  export PATH=\"$dir:\$PATH\"" ;;
esac
