#!/bin/sh
# Builds PR Nanny.app (menu-bar app + bundled Go helper) at DEST.
#
#   APP_VERSION=0.1.1-beta.1 SHORT_VERSION=0.1.1 BUILD_NUMBER=0.1.1 \
#     [SIGNING_IDENTITY='Developer ID Application: …'] [PROVISIONING_PROFILE=path] \
#     [ENTITLEMENTS=path] \
#     sh macos/Scripts/package-app.sh '/path/PR Nanny.app'
#   DEV_BUILD=1 sh macos/Scripts/package-app.sh '/path/PR Nanny Dev.app'
#
# APP_VERSION is the release version the helper reports; SHORT_VERSION and
# BUILD_NUMBER are the numeric CFBundleShortVersionString/CFBundleVersion.
# Without SIGNING_IDENTITY the bundle is ad-hoc signed: a local candidate
# only, never distributed. Dev builds use the .dev bundle ID and their own
# data directory, so they never touch the installed app's state.
# PROVISIONING_PROFILE is the Developer ID profile for com.alexcatdad.pr-nanny
# that authorizes macos/Resources/PRNanny.entitlements (data-protection
# keychain, team access group). It is embedded and those entitlements are used
# unless ENTITLEMENTS names others. Without it the app keeps the GitHub token
# in the login keychain with an app-only ACL instead.
# Notarization, stapling and publication belong to `task release`.
set -eu

DEST=${1:?usage: package-app.sh DEST.app}
case "$DEST" in *.app) ;; *) echo "DEST must end in .app" >&2; exit 2 ;; esac

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
MACOS="$ROOT/macos"
DEV_BUILD=${DEV_BUILD:-0}
SIGNING_IDENTITY=${SIGNING_IDENTITY:-}
ENTITLEMENTS=${ENTITLEMENTS:-}
PROVISIONING_PROFILE=${PROVISIONING_PROFILE:-}
TEAM_ID=CX6D6KGCT5
BASE_ID=com.alexcatdad.pr-nanny

if [ "$DEV_BUILD" = 1 ]; then
  BUNDLE_ID="$BASE_ID.dev"; NAME="PR Nanny Dev"
  APP_VERSION=${APP_VERSION:-dev}; SHORT_VERSION=${SHORT_VERSION:-0.0.0}; BUILD_NUMBER=${BUILD_NUMBER:-1}
else
  BUNDLE_ID="$BASE_ID"; NAME="PR Nanny"
  APP_VERSION=${APP_VERSION:?APP_VERSION is required for release builds}
  SHORT_VERSION=${SHORT_VERSION:?SHORT_VERSION is required for release builds}
  BUILD_NUMBER=${BUILD_NUMBER:-$SHORT_VERSION}
fi
printf '%s\n' "$SHORT_VERSION" | grep -Eq '^[0-9]+(\.[0-9]+){1,2}$' || { echo "invalid SHORT_VERSION: $SHORT_VERSION" >&2; exit 2; }
printf '%s\n' "$BUILD_NUMBER" | grep -Eq '^[0-9]+(\.[0-9]+){0,2}$' || { echo "invalid BUILD_NUMBER: $BUILD_NUMBER" >&2; exit 2; }
[ "$(uname -s)" = Darwin ] && [ "$(uname -m)" = arm64 ] || { echo "packaging requires an Apple Silicon Mac" >&2; exit 2; }

plist_get() { /usr/libexec/PlistBuddy -c "Print :$2" "$1" 2>/dev/null || true; }

# Never replace a bundle that belongs to something else.
if [ -e "$DEST" ]; then
  existing=$(plist_get "$DEST/Contents/Info.plist" CFBundleIdentifier)
  if [ "$existing" != "$BUNDLE_ID" ]; then
    echo "refusing to replace $DEST (bundle ID '${existing:-none}', expected $BUNDLE_ID)" >&2
    exit 1
  fi
fi

PARENT=$(dirname "$DEST")
mkdir -p "$PARENT"
STAGE=$(mktemp -d "$PARENT/.pr-nanny-stage.XXXXXX")
trap 'rm -rf "$STAGE"' EXIT HUP INT TERM
APP="$STAGE/app.app"

echo "==> Building menu-bar app (arm64)"
swift build -c release --arch arm64 --package-path "$MACOS" --product PRNanny -Xswiftc -warnings-as-errors >/dev/null
BIN=$(swift build -c release --arch arm64 --package-path "$MACOS" --show-bin-path)

echo "==> Building pr-nanny-helper $APP_VERSION"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Helpers" "$APP/Contents/Resources"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -C "$ROOT" -tags production -trimpath \
  -ldflags "-s -w -X main.version=$APP_VERSION" -o "$APP/Contents/Helpers/pr-nanny-helper" ./cmd/pr-nanny-helper

echo "==> Assembling $NAME"
cp "$BIN/PRNanny" "$APP/Contents/MacOS/PRNanny"
cp "$ROOT/build/darwin/icon.icns" "$APP/Contents/Resources/AppIcon.icns"
cp "$MACOS/Resources/PRNannyTrayIcon.png" "$MACOS/Resources/PRNannyTrayIcon@2x.png" "$APP/Contents/Resources/"
cp "$ROOT/LICENSE" "$APP/Contents/Resources/LICENSE"
cp "$MACOS/Resources/Info.plist" "$APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier $BUNDLE_ID" \
  -c "Set :CFBundleName $NAME" \
  -c "Set :CFBundleDisplayName $NAME" \
  -c "Set :CFBundleShortVersionString $SHORT_VERSION" \
  -c "Set :CFBundleVersion $BUILD_NUMBER" "$APP/Contents/Info.plist"
if [ -n "$PROVISIONING_PROFILE" ]; then
  [ "$DEV_BUILD" != 1 ] || { echo "dev builds are ad-hoc signed and cannot embed a provisioning profile" >&2; exit 2; }
  [ -n "$SIGNING_IDENTITY" ] || { echo "PROVISIONING_PROFILE needs SIGNING_IDENTITY" >&2; exit 2; }
  security cms -D -i "$PROVISIONING_PROFILE" > "$STAGE/profile.plist" || { echo "cannot decode $PROVISIONING_PROFILE" >&2; exit 1; }
  profile_app=$(plist_get "$STAGE/profile.plist" Entitlements:com.apple.application-identifier)
  [ "$profile_app" = "$TEAM_ID.$BUNDLE_ID" ] || { echo "profile is for '$profile_app', expected $TEAM_ID.$BUNDLE_ID" >&2; exit 1; }
  cp "$PROVISIONING_PROFILE" "$APP/Contents/embedded.provisionprofile"
  ENTITLEMENTS=${ENTITLEMENTS:-$MACOS/Resources/PRNanny.entitlements}
elif [ -n "$ENTITLEMENTS" ] && grep -q keychain-access-groups "$ENTITLEMENTS"; then
  # Restricted entitlements without a profile make macOS refuse to launch the app.
  echo "$ENTITLEMENTS needs PROVISIONING_PROFILE" >&2
  exit 2
fi
find "$APP" -type d -exec chmod 0755 {} +
find "$APP" -type f -exec chmod 0644 {} +
chmod 0755 "$APP/Contents/MacOS/PRNanny" "$APP/Contents/Helpers/pr-nanny-helper"

echo "==> Validating"
APP_VERSION="$APP_VERSION" sh "$MACOS/Scripts/validate-bundle.sh" "$APP"

echo "==> Signing"
if [ -n "$SIGNING_IDENTITY" ]; then
  set -- --force --options runtime --timestamp --sign "$SIGNING_IDENTITY"
else
  set -- --force --sign -
  echo "    (ad-hoc: local candidate only)"
fi
# The helper first, with a stable identifier. It has no keychain items of its
# own: the app keeps the GitHub token and hands it over the socket, and the
# helper only reads and deletes the login-keychain item PR Nanny 0.1.x wrote
# (whose ACL trusts /usr/bin/security, not this signature). The outer app last
# so its seal covers the finished helper.
codesign "$@" --identifier "$BUNDLE_ID.helper" "$APP/Contents/Helpers/pr-nanny-helper"
if [ -n "$ENTITLEMENTS" ]; then set -- "$@" --entitlements "$ENTITLEMENTS"; fi
codesign "$@" "$APP"
codesign --verify --deep --strict "$APP"

# Swap into place; restore the previous bundle if the move fails.
if [ -e "$DEST" ]; then
  OLD="$STAGE/previous.app"
  mv "$DEST" "$OLD"
  if ! mv "$APP" "$DEST"; then mv "$OLD" "$DEST"; echo "install failed; previous bundle restored" >&2; exit 1; fi
else
  mv "$APP" "$DEST"
fi
echo "==> $DEST"
