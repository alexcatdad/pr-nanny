#!/bin/sh
# Wrap an already signed/stapled release app; never rebuild or re-sign the app.
set -eu
archive=${1:?usage: package-dmg.sh signed-app.zip VERSION output-directory}
version=${2:?missing version}
output_dir=${3:?missing output directory}
: "${CODESIGN_IDENTITY:?Set the existing local Developer ID Application identity}"
[ "$(uname -s)" = Darwin ] || { echo 'DMG packaging requires macOS' >&2; exit 2; }
case "$CODESIGN_IDENTITY" in 'Developer ID Application: '*) ;; *) echo 'Developer ID Application identity required' >&2; exit 2;; esac
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
printf '%s\n' "$version" > "$work/VERSION"
sh "$root/scripts/read-version.sh" "$work/VERSION" >/dev/null
if unzip -Z1 "$archive" | grep -Eq '(^/|(^|/)\.\.(/|$)|\\)'; then
  echo 'Unsafe archive paths' >&2; exit 1
fi
mkdir "$work/contents"
ditto -x -k "$archive" "$work/contents"
rm -rf "$work/contents/__MACOSX"
app="$work/contents/PR Nanny.app"
[ -d "$app" ] && [ ! -L "$app" ] || { echo 'Expected app bundle missing' >&2; exit 1; }
codesign --verify --deep --strict --test-requirement '=anchor apple generic and certificate leaf[subject.OU] = "CX6D6KGCT5"' "$app"
xcrun stapler validate "$app"
actual=$("$app/Contents/MacOS/pr-nanny" --version)
case "$actual" in "PR Nanny $version ("*")") ;; *) echo 'App version does not match requested version' >&2; exit 1;; esac
ln -s /Applications "$work/contents/Applications"
cat > "$work/contents/Install PR Nanny.txt" <<'EOF'
Drag PR Nanny.app onto Applications.
Eject this disk image, then open PR Nanny from Applications.
PR Nanny appears in the menu bar. Open Settings there to connect GitHub.
EOF
mkdir -p "$output_dir"
output="$output_dir/pr-nanny-$version-darwin-arm64.dmg"
[ ! -e "$output" ] || { echo 'Refusing to overwrite an existing DMG' >&2; exit 1; }
hdiutil create -quiet -fs HFS+ -volname 'PR Nanny' -srcfolder "$work/contents" -format UDZO "$output"
codesign --force --timestamp --sign "$CODESIGN_IDENTITY" --identifier io.github.alexcatdad.pr-nanny.dmg "$output"
codesign --verify --strict "$output"
echo "Created $output. Notarise and staple the DMG before computing its checksum."
