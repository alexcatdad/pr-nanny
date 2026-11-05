#!/bin/sh
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
DIST=${1:-"$ROOT/dist"}
VERSION=$(sh "$ROOT/scripts/read-version.sh")
found=0
verify_build_tags() {
  binary=$1
  buildinfo=$(go version -m "$binary") || { echo "cannot inspect Go build metadata for $binary" >&2; return 1; }
  printf '%s\n' "$buildinfo" | grep -q -- '-tags=production' || { echo "production build tag missing from $binary" >&2; return 1; }
  if printf '%s\n' "$buildinfo" | grep -Eiq -- '-tags=[^[:space:]]*(mcp|devtools|server)'; then
    echo "forbidden development build tag found in $binary" >&2; return 1
  fi
  case "$(uname -s):$name" in
    Darwin:*-darwin-arm64.zip)
      chmod u+x "$binary"
      actual=$("$binary" --version)
      case "$actual" in "PR Nanny $VERSION ("*")") ;; *) echo 'Runtime version does not match VERSION' >&2; return 1;; esac
      ;;
  esac
}
for checksum in "$DIST"/*.sha256; do
  [ -f "$checksum" ] || continue
  archive=${checksum%.sha256}
  [ -f "$archive" ] || { echo "missing archive for $checksum" >&2; exit 1; }
  (
    cd "$DIST"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum -c "$(basename "$checksum")"
    else
      shasum -a 256 -c "$(basename "$checksum")"
    fi
  )
  name=$(basename "$archive")
  listing=""
  case "$name" in
    *-darwin-arm64.zip)
      listing=$(unzip -Z1 "$archive")
      ;;
    *) echo "unrecognized release archive: $name" >&2; exit 1;;
  esac
  if printf '%s\n' "$listing" | grep -Eq '(^/|(^|/)\.\.(/|$)|\\\.\\.)'; then
    echo "unsafe archive path in $name" >&2
    exit 1
  fi
  case "$name" in
    *-darwin-arm64.zip)
      printf '%s\n' "$listing" | grep -q '^PR Nanny.app/Contents/MacOS/pr-nanny$'
      printf '%s\n' "$listing" | grep -q '^PR Nanny.app/Contents/Info.plist$'
      temp=$(mktemp -d)
      trap 'rm -rf "$temp"' EXIT HUP INT TERM
      unzip -q "$archive" 'PR Nanny.app/Contents/Info.plist' -d "$temp"
      plist="$temp/PR Nanny.app/Contents/Info.plist"
      /usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist" | grep -qx 'io.github.alexcatdad.pr-nanny'
      /usr/libexec/PlistBuddy -c 'Print :LSUIElement' "$plist" | grep -qx 'true'
      /usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$plist" | grep -qx '13.0'
      unzip -q "$archive" 'PR Nanny.app/Contents/MacOS/pr-nanny' -d "$temp"
      file "$temp/PR Nanny.app/Contents/MacOS/pr-nanny" | grep -q 'arm64'
      verify_build_tags "$temp/PR Nanny.app/Contents/MacOS/pr-nanny"
      rm -rf "$temp"
      trap - EXIT HUP INT TERM
      ;;
  esac
  found=1
done
[ "$found" -eq 1 ] || { echo "no package checksum files found in $DIST" >&2; exit 1; }
echo "Package checks passed."
