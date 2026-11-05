#!/bin/sh
# Structural checks for a PR Nanny.app bundle (signed or not).
#   [APP_VERSION=0.1.1-beta.1] sh macos/Scripts/validate-bundle.sh '/path/PR Nanny.app'
set -eu
APP=${1:?usage: validate-bundle.sh APP}
C="$APP/Contents"
fail() { echo "validate-bundle: $*" >&2; exit 1; }
get() { /usr/libexec/PlistBuddy -c "Print :$2" "$1" 2>/dev/null || true; }

plutil -lint -s "$C/Info.plist" || fail "Info.plist does not lint"
ID=$(get "$C/Info.plist" CFBundleIdentifier)
case "$ID" in
  com.alexcatdad.pr-nanny|com.alexcatdad.pr-nanny.dev) ;;
  *) fail "unexpected bundle ID '$ID'" ;;
esac
[ "$(get "$C/Info.plist" CFBundleExecutable)" = PRNanny ] || fail "CFBundleExecutable must be PRNanny"
[ "$(get "$C/Info.plist" LSUIElement)" = true ] || fail "LSUIElement must be true"
[ "$(get "$C/Info.plist" LSMinimumSystemVersion)" = 13.0 ] || fail "LSMinimumSystemVersion must be 13.0"

for exe in MacOS/PRNanny Helpers/pr-nanny-helper; do
  [ -f "$C/$exe" ] && [ -x "$C/$exe" ] || fail "missing executable $exe"
  file "$C/$exe" | grep -q 'arm64' || fail "$exe is not arm64"
done
for resource in AppIcon.icns PRNannyTrayIcon.png PRNannyTrayIcon@2x.png LICENSE; do
  [ -f "$C/Resources/$resource" ] || fail "missing Resources/$resource"
done
[ "$(sips -g pixelWidth "$C/Resources/PRNannyTrayIcon.png" | awk '/pixelWidth/ {print $2}')" = 18 ] || fail "tray icon must be 18 px"
[ "$(sips -g pixelWidth "$C/Resources/PRNannyTrayIcon@2x.png" | awk '/pixelWidth/ {print $2}')" = 36 ] || fail "@2x tray icon must be 36 px"
[ "$(find "$C" -type f | wc -l | tr -d ' ')" -le 9 ] || fail "unexpected extra files in the bundle"

# The helper is a production build without development tags.
info=$(go version -m "$C/Helpers/pr-nanny-helper") || fail "cannot read helper build metadata"
printf '%s\n' "$info" | grep -Eq 'build[[:space:]]+-tags=production$' || fail "helper lacks the production build tag"
if printf '%s\n' "$info" | grep -Eiq -- '-tags=[^[:space:]]*(mcp|devtools|server)'; then fail "helper has development build tags"; fi
if printf '%s\n' "$info" | grep -q 'wailsapp/wails'; then fail "helper links Wails"; fi
version=$("$C/Helpers/pr-nanny-helper" -version)
case "$version" in "pr-nanny-helper ${APP_VERSION:-}"*" protocol 1") ;; *) fail "helper reports '$version'" ;; esac
echo "validate-bundle: ok ($ID, $version)"
