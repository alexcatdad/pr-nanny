#!/bin/sh
# Pinned lint tools for macOS arm64 (local and CI) and Linux x64 (Codespaces).
# Digests are the upstream GitHub release asset digests.
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
case "$(uname -s)-$(uname -m)" in
  Darwin-arm64)
    zizmor_asset=zizmor-aarch64-apple-darwin.tar.gz
    zizmor_digest=ad5aa633cd56215b11e6c51d51f90c7e6b22904a75047d1215abbd422f61d698
    shellcheck_asset=shellcheck-v0.11.0.darwin.aarch64.tar.xz
    shellcheck_digest=56affdd8de5527894dca6dc3d7e0a99a873b0f004d7aabc30ae407d3f48b0a79
    ;;
  Linux-x86_64)
    zizmor_asset=zizmor-x86_64-unknown-linux-gnu.tar.gz
    zizmor_digest=d185f726bdaf92df78273217dfdd24ebcd6b0ef48cf3ce776ff5762b6e138c99
    shellcheck_asset=shellcheck-v0.11.0.linux.x86_64.tar.xz
    shellcheck_digest=8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198
    ;;
  *) echo 'lint tool bootstrap supports macOS arm64 and Linux x64 only' >&2; exit 2;;
esac
stamp="$root/.tools/.ci-tools-$zizmor_digest-$shellcheck_digest"
[ -f "$stamp" ] && exit 0
mkdir -p "$root/.tools"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
fetch() {
  name=$1 url=$2 digest=$3
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$url" -o "$work/$name"
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s  %s\n' "$digest" "$work/$name" | sha256sum -c - >/dev/null
  else
    printf '%s  %s\n' "$digest" "$work/$name" | shasum -a 256 -c - >/dev/null
  fi
}
fetch zizmor.tar.gz "https://github.com/zizmorcore/zizmor/releases/download/v1.31.1/$zizmor_asset" "$zizmor_digest"
fetch shellcheck.tar.xz "https://github.com/koalaman/shellcheck/releases/download/v0.11.0/$shellcheck_asset" "$shellcheck_digest"
tar -xzf "$work/zizmor.tar.gz" -C "$work" zizmor
tar -xJf "$work/shellcheck.tar.xz" -C "$work" shellcheck-v0.11.0/shellcheck
install -m 755 "$work/zizmor" "$work/shellcheck-v0.11.0/shellcheck" "$root/.tools/"
rm -f "$root"/.tools/.ci-tools-*
touch "$stamp"
