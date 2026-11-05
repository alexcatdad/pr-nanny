#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
SOURCE_VERSION=$(sh "$ROOT/scripts/read-version.sh")
VERSION=${1:-$SOURCE_VERSION}
TARGET=${2:-$(uname -s | tr '[:upper:]' '[:lower:]')}
[ "$VERSION" = "$SOURCE_VERSION" ] || { echo "requested version $VERSION does not match VERSION ($SOURCE_VERSION)" >&2; exit 2; }
mkdir -p "$ROOT/dist" "$ROOT/bin"

build_binary() {
  output=$1
  shift
  if [ "${PR_NANNY_PREBUILT_FRONTEND:-0}" = 1 ]; then
    [ -f "$ROOT/frontend/dist/index.html" ] || { echo "prebuilt frontend is missing frontend/dist/index.html" >&2; exit 1; }
  else
    (cd "$ROOT/frontend" && pnpm run build)
  fi
  (cd "$ROOT" && CGO_ENABLED=1 GOOS="$GOOS" GOARCH="$GOARCH" MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0" go build -tags production -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$output" .)
}

checksum() {
  archive=$1
  (
    cd "$(dirname "$archive")"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$(basename "$archive")" > "$(basename "$archive").sha256"
    else
      shasum -a 256 "$(basename "$archive")" > "$(basename "$archive").sha256"
    fi
  )
}

case "$TARGET" in
  darwin|macos)
    [ "$(uname -s)" = Darwin ] || { echo "macOS packaging requires a macOS host and native WebKit frameworks" >&2; exit 2; }
    [ "$(uname -m)" = arm64 ] || { echo "macOS ARM64 packaging requires an Apple Silicon host" >&2; exit 2; }
    [ -f "$ROOT/build/darwin/icon.icns" ] || { echo "missing build/darwin/icon.icns" >&2; exit 1; }
    GOOS=darwin GOARCH=arm64
    SHORT_VERSION=${MACOS_SHORT_VERSION:-${VERSION%%[-+]*}}
    BUNDLE_VERSION=${MACOS_BUNDLE_VERSION:-$SHORT_VERSION}
    printf '%s\n' "$SHORT_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "macOS bundle version must begin with numeric major.minor.patch" >&2; exit 2; }
    binary="$ROOT/bin/pr-nanny"
    build_binary "$binary"
    stage=$(mktemp -d "$ROOT/dist/.package.XXXXXX")
    trap 'rm -rf "$stage"' EXIT HUP INT TERM
    app="$stage/PR Nanny.app"
    mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
    install -m 755 "$binary" "$app/Contents/MacOS/pr-nanny"
    install -m 644 "$ROOT/build/darwin/icon.icns" "$app/Contents/Resources/AppIcon.icns"
    install -m 644 "$ROOT/LICENSE" "$app/Contents/Resources/LICENSE"
    cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>pr-nanny</string>
<key>CFBundleIdentifier</key><string>io.github.alexcatdad.pr-nanny</string>
<key>CFBundleName</key><string>PR Nanny</string>
<key>CFBundleDisplayName</key><string>PR Nanny</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>$SHORT_VERSION</string>
<key>CFBundleVersion</key><string>$BUNDLE_VERSION</string>
<key>LSUIElement</key><true/>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>CFBundleIconFile</key><string>AppIcon</string>
</dict></plist>
EOF
    if [ -n "${MACOS_SIGNING_IDENTITY:-}" ]; then
      # Local release: Developer ID, hardened runtime and secure timestamp.
      set -- --force --options runtime --timestamp --sign "$MACOS_SIGNING_IDENTITY"
      if [ -n "${MACOS_ENTITLEMENTS:-}" ]; then set -- "$@" --entitlements "$MACOS_ENTITLEMENTS"; fi
      codesign "$@" "$app/Contents/MacOS/pr-nanny"
      codesign "$@" "$app"
      codesign --verify --deep --strict "$app"
    else
      codesign --force --deep --sign - "$app"
    fi
    archive="$ROOT/dist/pr-nanny-$VERSION-darwin-arm64.zip"
    rm -f "$archive"
    ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
    checksum "$archive"
    echo "Created app archive: $archive"
    ;;
  *) echo "usage: $0 [version] [darwin]" >&2; exit 2;;
esac
