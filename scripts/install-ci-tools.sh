#!/bin/sh
# Pinned Linux/x64 tools. Digests verified against upstream release metadata.
set -eu
[ "$(uname -s)-$(uname -m)" = Linux-x86_64 ] || { echo 'CI tool bootstrap requires Linux x64' >&2; exit 2; }
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$root/.tools"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
fetch() {
  name=$1 url=$2 digest=$3
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$url" -o "$work/$name"
  printf '%s  %s\n' "$digest" "$work/$name" | sha256sum -c -
}
fetch gitleaks.tar.gz https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz 551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
fetch zizmor.tar.gz https://github.com/zizmorcore/zizmor/releases/download/v1.31.1/zizmor-x86_64-unknown-linux-gnu.tar.gz d185f726bdaf92df78273217dfdd24ebcd6b0ef48cf3ce776ff5762b6e138c99
fetch shellcheck.tar.xz https://github.com/koalaman/shellcheck/releases/download/v0.11.0/shellcheck-v0.11.0.linux.x86_64.tar.xz 8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198
tar -xzf "$work/gitleaks.tar.gz" -C "$work" gitleaks
tar -xzf "$work/zizmor.tar.gz" -C "$work" zizmor
tar -xJf "$work/shellcheck.tar.xz" -C "$work" shellcheck-v0.11.0/shellcheck
install -m 755 "$work/gitleaks" "$work/zizmor" "$work/shellcheck-v0.11.0/shellcheck" "$root/.tools/"
