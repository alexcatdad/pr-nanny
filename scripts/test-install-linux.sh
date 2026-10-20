#!/bin/sh
# Exercise the release install/uninstall scripts in isolated XDG directories.
set -eu
archive=${1:?usage: test-install-linux.sh archive.tar.gz}
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
version=$(sh "$root/scripts/read-version.sh")
[ "$(uname -s)" = Linux ] || { echo 'Linux package install tests require Linux' >&2; exit 2; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/unpacked"
tar -xzf "$archive" -C "$work/unpacked"
set -- "$work"/unpacked/pr-nanny-*
payload=$1
export XDG_DATA_HOME="$work/data with spaces"
export XDG_BIN_HOME="$work/bin with spaces"
sh "$payload/install.sh"
test -x "$XDG_BIN_HOME/pr-nanny"
test -f "$XDG_DATA_HOME/applications/pr-nanny.desktop"
test -f "$XDG_DATA_HOME/icons/hicolor/256x256/apps/pr-nanny.png"
grep -Fqx "Exec=\"$XDG_BIN_HOME/pr-nanny\"" "$XDG_DATA_HOME/applications/pr-nanny.desktop"
actual=$("$XDG_BIN_HOME/pr-nanny" --version)
case "$actual" in "PR Nanny $version ("*")") ;; *) echo 'Installed binary version mismatch' >&2; exit 1;; esac
# Reinstallation is supported and uninstall keeps unrelated data.
printf '%s\n' synthetic > "$XDG_DATA_HOME/keep-me"
sh "$payload/install.sh"
sh "$payload/uninstall.sh"
test ! -e "$XDG_BIN_HOME/pr-nanny"
test ! -e "$XDG_DATA_HOME/applications/pr-nanny.desktop"
test ! -e "$XDG_DATA_HOME/icons/hicolor/256x256/apps/pr-nanny.png"
test -f "$XDG_DATA_HOME/keep-me"
echo 'Isolated Linux install/uninstall passed.'
