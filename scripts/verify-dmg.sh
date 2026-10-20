#!/bin/sh
set -eu
image=${1:?usage: verify-dmg.sh release.dmg VERSION reference-signed-app.zip}
version=${2:?missing version}
reference=${3:?missing reference ZIP}
[ "$(uname -s)" = Darwin ] || { echo 'DMG verification requires macOS' >&2; exit 2; }
work=$(mktemp -d)
mount_dir="$work/image"
attached=false
cleanup() {
  result=$?
  trap - EXIT HUP INT TERM
  if [ "$attached" = true ]; then
    if ! hdiutil detach "$mount_dir" -quiet; then
      echo "Could not detach owned image; preserving $work" >&2
      exit 1
    fi
  fi
  rm -rf "$work"
  exit "$result"
}
trap cleanup EXIT HUP INT TERM
hdiutil verify "$image"
codesign --verify --strict --test-requirement '=anchor apple generic and certificate leaf[subject.OU] = "CX6D6KGCT5"' "$image"
xcrun stapler validate "$image"
spctl --assess --type open --context context:primary-signature --verbose=2 "$image"
mkdir "$mount_dir" "$work/reference"
attached=true
hdiutil attach "$image" -readonly -nobrowse -noautoopen -mountpoint "$mount_dir" > "$work/attach.log"
[ -L "$mount_dir/Applications" ] && [ "$(readlink "$mount_dir/Applications")" = /Applications ]
app="$mount_dir/PR Nanny.app"
codesign --verify --deep --strict --test-requirement '=anchor apple generic and certificate leaf[subject.OU] = "CX6D6KGCT5"' "$app"
xcrun stapler validate "$app"
spctl --assess --type execute --verbose=2 "$app"
actual=$("$app/Contents/MacOS/pr-nanny" --version)
case "$actual" in "PR Nanny $version ("*")") ;; *) echo 'Contained app version mismatch' >&2; exit 1;; esac
if unzip -Z1 "$reference" | grep -Eq '(^/|(^|/)\.\.(/|$)|\\)'; then
  echo 'Unsafe reference archive paths' >&2; exit 1
fi
ditto -x -k "$reference" "$work/reference"
diff -r "$work/reference/PR Nanny.app" "$app"
echo 'Verified notarised DMG, Applications link, app version and exact reference-app contents.'
