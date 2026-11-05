#!/bin/sh
# Checks every dist/*.zip.sha256 pair: checksum, safe paths, bundle layout,
# bundle ID, signature order and the helper's production build.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
DIST=${1:-"$ROOT/dist"}
VERSION=$(sh "$ROOT/scripts/read-version.sh")
found=0
for checksum in "$DIST"/*.sha256; do
  [ -f "$checksum" ] || continue
  archive=${checksum%.sha256}
  [ -f "$archive" ] || { echo "missing archive for $checksum" >&2; exit 1; }
  (cd "$DIST" && shasum -a 256 -c "$(basename "$checksum")")
  name=$(basename "$archive")
  case "$name" in *-darwin-arm64.zip) ;; *) echo "unrecognized release archive: $name" >&2; exit 1;; esac
  listing=$(unzip -Z1 "$archive")
  if printf '%s\n' "$listing" | grep -Eq '(^/|(^|/)\.\.(/|$)|\\)'; then
    echo "unsafe archive path in $name" >&2; exit 1
  fi
  for entry in 'PR Nanny.app/Contents/Info.plist' 'PR Nanny.app/Contents/MacOS/PRNanny' \
    'PR Nanny.app/Contents/Helpers/pr-nanny-helper' 'PR Nanny.app/Contents/_CodeSignature/CodeResources'; do
    printf '%s\n' "$listing" | grep -qxF "$entry" || { echo "$name lacks $entry" >&2; exit 1; }
  done
  temp=$(mktemp -d)
  trap 'rm -rf "$temp"' EXIT HUP INT TERM
  ditto -x -k "$archive" "$temp"
  app="$temp/PR Nanny.app"
  /usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Contents/Info.plist" | grep -qx 'com.alexcatdad.pr-nanny'
  codesign --verify --deep --strict "$app"
  codesign --verify --strict "$app/Contents/Helpers/pr-nanny-helper"
  APP_VERSION="$VERSION" sh "$ROOT/macos/Scripts/validate-bundle.sh" "$app"
  rm -rf "$temp"
  trap - EXIT HUP INT TERM
  found=1
done
[ "$found" -eq 1 ] || { echo "no package checksum files found in $DIST" >&2; exit 1; }
echo "Package checks passed."
