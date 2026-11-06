#!/bin/sh
# Builds dist/pr-nanny-VERSION-darwin-arm64.zip containing PR Nanny.app
# (Swift menu-bar app + Contents/Helpers/pr-nanny-helper) and its .sha256.
#   sh scripts/package.sh [VERSION] [darwin]
# MACOS_SIGNING_IDENTITY (Developer ID), MACOS_PROVISIONING_PROFILE (see
# build/SIGNING.md) and MACOS_ENTITLEMENTS are optional;
# without an identity the bundle is ad-hoc signed and only a local candidate.
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
SOURCE_VERSION=$(sh "$ROOT/scripts/read-version.sh")
VERSION=${1:-$SOURCE_VERSION}
TARGET=${2:-darwin}
[ "$VERSION" = "$SOURCE_VERSION" ] || { echo "requested version $VERSION does not match VERSION ($SOURCE_VERSION)" >&2; exit 2; }
case "$TARGET" in darwin|macos) ;; *) echo "usage: $0 [version] [darwin]" >&2; exit 2;; esac
[ -f "$ROOT/build/darwin/icon.icns" ] || { echo "missing build/darwin/icon.icns" >&2; exit 1; }

SHORT_VERSION=${MACOS_SHORT_VERSION:-${VERSION%%[-+]*}}
BUNDLE_VERSION=${MACOS_BUNDLE_VERSION:-$SHORT_VERSION}
mkdir -p "$ROOT/dist"
stage=$(mktemp -d "$ROOT/dist/.package.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
app="$stage/PR Nanny.app"
APP_VERSION="$VERSION" SHORT_VERSION="$SHORT_VERSION" BUILD_NUMBER="$BUNDLE_VERSION" \
  SIGNING_IDENTITY="${MACOS_SIGNING_IDENTITY:-}" ENTITLEMENTS="${MACOS_ENTITLEMENTS:-}" \
  PROVISIONING_PROFILE="${MACOS_PROVISIONING_PROFILE:-}" \
  sh "$ROOT/macos/Scripts/package-app.sh" "$app"

archive="$ROOT/dist/pr-nanny-$VERSION-darwin-arm64.zip"
rm -f "$archive" "$archive.sha256"
ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
(cd "$ROOT/dist" && shasum -a 256 "$(basename "$archive")" > "$(basename "$archive").sha256")
echo "Created app archive: $archive"
