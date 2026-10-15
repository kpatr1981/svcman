#!/bin/sh
# svcman installer for Linux and macOS — Author: Konstantinos Patronas <kpatronas@gmail.com>
#
# From a source checkout with Go installed it builds svcman; otherwise it downloads the
# release binary for this OS/CPU from GitHub and checks it against SHA256SUMS.
#
#   curl -fsSL https://raw.githubusercontent.com/kpatr1981/svcman/main/install.sh | sh
#   sh install.sh                     install to ~/.local/bin (no sudo)
#   sh install.sh /usr/local/bin      another directory (sudo if it is not writable)
#   sh install.sh --download [DIR]    use the release binary even in a checkout
#   sh install.sh --uninstall [DIR]   remove it
#
# SVCMAN_VERSION=v0.1.0 picks a release (default: the latest).
set -eu

repo=kpatr1981/svcman
mode=auto
case "${1:-}" in
  --uninstall) mode=uninstall; shift ;;
  --download) mode=download; shift ;;
  -h|--help) sed -n '2,13p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'; exit 0 ;;
esac
dir=${1:-$HOME/.local/bin}
target=$dir/svcman

say() { printf '%s\n' "$*"; }
die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

as_needed() { # run as root only when the directory is not writable by us
  if [ -w "$dir" ] || { [ ! -e "$dir" ] && mkdir -p "$dir" 2>/dev/null; }; then "$@"; else sudo "$@"; fi
}

if [ $mode = uninstall ]; then
  [ -e "$target" ] || { say "svcman is not installed in $dir"; exit 0; }
  as_needed rm -f "$target"
  say "removed $target"
  exit 0
fi

src=$(cd "$(dirname "$0")" 2>/dev/null && pwd)
tmpd=$(mktemp -d)
trap 'rm -rf "$tmpd"' EXIT

if [ $mode = auto ] && [ -f "$src/main.go" ] && [ -f "$src/go.mod" ] && command -v go >/dev/null 2>&1; then
  version=$(sed -n 's/^VERSION *?= *//p' "$src/Makefile")
  say "building svcman ${version} from $src"
  (cd "$src" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$tmpd/svcman" .)
else
  case $(uname -s) in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "unsupported OS $(uname -s); on Windows use install.ps1" ;;
  esac
  case $(uname -m) in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die "unsupported CPU $(uname -m); build from source (Go 1.26+)" ;;
  esac
  ver=${SVCMAN_VERSION:-latest}
  if [ "$ver" = latest ]; then base=https://github.com/$repo/releases/latest/download
  else base=https://github.com/$repo/releases/download/$ver; fi
  asset=svcman-$os-$arch
  fetch() { # url file
    if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then wget -q -O "$2" "$1"
    else die "needs curl or wget"; fi
  }
  say "downloading $asset ($ver)"
  fetch "$base/$asset" "$tmpd/$asset" || die "download failed: $base/$asset"
  fetch "$base/SHA256SUMS" "$tmpd/SHA256SUMS" || die "download failed: $base/SHA256SUMS"
  want=$(awk -v f="$asset" '$2 == f {print $1}' "$tmpd/SHA256SUMS")
  if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmpd/$asset" | awk '{print $1}')
  else got=$(shasum -a 256 "$tmpd/$asset" | awk '{print $1}'); fi
  [ -n "$want" ] && [ "$want" = "$got" ] || die "checksum mismatch for $asset (want ${want:-none}, got $got)"
  say "checksum ok"
  mv "$tmpd/$asset" "$tmpd/svcman"
fi

as_needed mkdir -p "$dir"
as_needed install -m 0755 "$tmpd/svcman" "$target"
say "installed $target"
"$target" --version | head -1

case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "note: $dir is not in your PATH; add it, e.g.  export PATH=\"$dir:\$PATH\"" ;;
esac
command -v ssh >/dev/null 2>&1 || say "note: svcman needs the ssh client (openssh-client) to reach hosts"
