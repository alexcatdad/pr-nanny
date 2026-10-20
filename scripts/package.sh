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
  if [ "$GOOS" = darwin ]; then
    (cd "$ROOT" && CGO_ENABLED=1 GOOS="$GOOS" GOARCH="$GOARCH" MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0" go build -tags production -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$output" .)
  else
    (cd "$ROOT" && CGO_ENABLED=1 GOOS="$GOOS" GOARCH="$GOARCH" go build -tags production -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$output" .)
  fi
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
    SHORT_VERSION=${VERSION%%[-+]*}
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
<key>CFBundleVersion</key><string>$SHORT_VERSION</string>
<key>LSUIElement</key><true/>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>CFBundleIconFile</key><string>AppIcon</string>
</dict></plist>
EOF
    codesign --force --deep --sign - "$app"
    archive="$ROOT/dist/pr-nanny-$VERSION-darwin-arm64.zip"
    rm -f "$archive"
    ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
    checksum "$archive"
    echo "Created unsigned/ad-hoc-signed app archive: $archive"
    ;;
  linux)
    [ "$(uname -s)" = Linux ] || { echo "Linux packaging requires a Linux host with GTK4 and WebKitGTK 6 development files" >&2; exit 2; }
    [ "$(uname -m)" = x86_64 ] || { echo "Linux x64 packaging requires an x86_64 host" >&2; exit 2; }
    if ! command -v pkg-config >/dev/null || ! pkg-config --exists gtk4 webkitgtk-6.0; then
      echo "install GTK4 and WebKitGTK 6 development packages" >&2
      exit 2
    fi
    [ -f "$ROOT/assets/icon.png" ] || { echo "missing assets/icon.png" >&2; exit 1; }
    GOOS=linux GOARCH=amd64
    binary="$ROOT/bin/pr-nanny"
    build_binary "$binary"
    stage=$(mktemp -d "$ROOT/dist/.package.XXXXXX")
    trap 'rm -rf "$stage"' EXIT HUP INT TERM
    root="$stage/pr-nanny-$VERSION"
    mkdir -p "$root/bin" "$root/share/applications" "$root/share/icons/hicolor/256x256/apps" "$root/scripts"
    install -m 755 "$binary" "$root/bin/pr-nanny"
    install -m 644 "$ROOT/LICENSE" "$root/LICENSE"
    install -m 755 "$ROOT/scripts/apparmor-profile.sh" "$root/scripts/apparmor-profile.sh"
    install -m 644 "$ROOT/build/linux/APPARMOR.md" "$root/APPARMOR.md"
    cat > "$root/README.txt" <<EOF
PR Nanny $VERSION for Linux x64

Requirements: GTK4, WebKitGTK 6.0, a Secret Service-compatible credential
store (for example GNOME Keyring or KWallet with Secret Service enabled), and
an XDG desktop session. Run ./install.sh to install for the current user.
Run ./uninstall.sh to remove the installed files.
Ubuntu 24.04+ may require the exact-path AppArmor allowance in APPARMOR.md.
EOF
    cp "$ROOT/assets/icon.png" "$root/share/icons/hicolor/256x256/apps/pr-nanny.png"
    cat > "$root/share/applications/pr-nanny.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=PR Nanny
Comment=Passively monitor your GitHub pull requests
Exec=pr-nanny
Icon=pr-nanny
Terminal=false
Categories=Development;Utility;
StartupNotify=true
EOF
    cat > "$root/install.sh" <<'EOF'
#!/bin/sh
set -eu
HERE=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
DATA_HOME=${XDG_DATA_HOME:-"$HOME/.local/share"}
BIN_HOME=${XDG_BIN_HOME:-"$HOME/.local/bin"}
mkdir -p "$DATA_HOME/applications" "$DATA_HOME/icons/hicolor/256x256/apps" "$BIN_HOME"
install -m 755 "$HERE/bin/pr-nanny" "$BIN_HOME/pr-nanny"
sed '/^Exec=/d' "$HERE/share/applications/pr-nanny.desktop" > "$DATA_HOME/applications/pr-nanny.desktop"
desktop_exec=$(printf '%s' "$BIN_HOME/pr-nanny" | sed 's/[\\"`$]/\\&/g;s/%/%%/g')
printf 'Exec="%s"\n' "$desktop_exec" >> "$DATA_HOME/applications/pr-nanny.desktop"
chmod 644 "$DATA_HOME/applications/pr-nanny.desktop"
install -m 644 "$HERE/share/icons/hicolor/256x256/apps/pr-nanny.png" "$DATA_HOME/icons/hicolor/256x256/apps/pr-nanny.png"
printf 'Installed for this user. Ensure %s is on PATH.\n' "$BIN_HOME"
EOF
    cat > "$root/uninstall.sh" <<'EOF'
#!/bin/sh
set -eu
DATA_HOME=${XDG_DATA_HOME:-"$HOME/.local/share"}
BIN_HOME=${XDG_BIN_HOME:-"$HOME/.local/bin"}
rm -f "$BIN_HOME/pr-nanny" "$DATA_HOME/applications/pr-nanny.desktop" "$DATA_HOME/icons/hicolor/256x256/apps/pr-nanny.png"
printf 'Removed PR Nanny application files. User settings and credentials were kept.\n'
EOF
    chmod 755 "$root/install.sh" "$root/uninstall.sh"
    archive="$ROOT/dist/pr-nanny-$VERSION-linux-x64.tar.gz"
    rm -f "$archive"
    tar -C "$stage" -czf "$archive" "pr-nanny-$VERSION"
    checksum "$archive"
    echo "Created Linux archive: $archive"
    ;;
  *) echo "usage: $0 [version] [darwin|linux]" >&2; exit 2;;
esac
