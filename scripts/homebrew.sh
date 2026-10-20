#!/bin/sh
set -eu
mode=both
case "${1:-}" in formula|cask|both) mode=$1; shift;; esac
usage() {
  echo "usage: $0 [formula|cask|both] VERSION [MAC_ZIP_URL MAC_SHA256] [LINUX_TAR_URL LINUX_SHA256]" >&2
  echo "  cask:    $0 cask VERSION MAC_ZIP_URL MAC_SHA256" >&2
  echo "  formula: $0 formula VERSION LINUX_TAR_URL LINUX_SHA256" >&2
  echo "  both:    $0 both VERSION MAC_ZIP_URL MAC_SHA256 LINUX_TAR_URL LINUX_SHA256" >&2
  exit 2
}
case "$mode" in
  cask) [ "$#" -eq 3 ] || usage; VERSION=$1 MAC_URL=$2 MAC_SHA=$3 ;;
  formula) [ "$#" -eq 3 ] || usage; VERSION=$1 LINUX_URL=$2 LINUX_SHA=$3 ;;
  both) [ "$#" -eq 5 ] || usage; VERSION=$1 MAC_URL=$2 MAC_SHA=$3 LINUX_URL=$4 LINUX_SHA=$5 ;;
esac
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
SOURCE_VERSION=$(sh "$ROOT/scripts/read-version.sh")
[ "$VERSION" = "$SOURCE_VERSION" ] || { echo "requested version $VERSION does not match VERSION ($SOURCE_VERSION)" >&2; exit 2; }
validate_archive() {
  url=$1 sha=$2
  case "$url" in https://*) ;; *) echo "archive URLs must use https" >&2; exit 2;; esac
  case "$url" in *[!A-Za-z0-9:/._+-]*) echo "URLs must not contain shell-sensitive or escaped characters" >&2; exit 2;; esac
  case "$sha" in *[!0-9A-Fa-f]*|'') echo "SHA256 values must contain exactly 64 hexadecimal characters" >&2; exit 2;; esac
  [ "${#sha}" -eq 64 ] || { echo "SHA256 values must contain exactly 64 hexadecimal characters" >&2; exit 2; }
}
if [ "$mode" = cask ] || [ "$mode" = both ]; then validate_archive "$MAC_URL" "$MAC_SHA"; fi
if [ "$mode" = formula ] || [ "$mode" = both ]; then validate_archive "$LINUX_URL" "$LINUX_SHA"; fi

if [ "$mode" = formula ] || [ "$mode" = both ]; then
cat <<EOF
# Save as Formula/pr-nanny.rb in a tap.
class PrNanny < Formula
  desc "Passive GitHub pull-request monitor"
  homepage "https://github.com/alexcatdad/pr-nanny"
  url "$LINUX_URL"
  version "$VERSION"
  sha256 "$LINUX_SHA"
  license "MIT"

  depends_on :linux

  def install
    odie "PR Nanny Linux packages require x86_64" unless Hardware::CPU.intel?
    bin.install "bin/pr-nanny"
    (share/"applications").install "share/applications/pr-nanny.desktop"
    (share/"icons/hicolor/256x256/apps").install "share/icons/hicolor/256x256/apps/pr-nanny.png"
    pkgshare.install "scripts/apparmor-profile.sh", "APPARMOR.md"
  end

  def caveats
    <<~EOS
      Requires the distribution's GTK4 and WebKitGTK 6.0 runtime libraries and glibc 2.39 or newer.
      Linux packages are built on Ubuntu 24.04. Native Windows is not supported by Homebrew.
    EOS
  end

  test do
    assert_predicate bin/"pr-nanny", :executable?
  end
end
EOF
fi

if [ "$mode" = cask ] || [ "$mode" = both ]; then
[ "$mode" = both ] && printf '\n'
cat <<EOF
# Save as Casks/pr-nanny.rb in a tap.
cask "pr-nanny" do
  version "$VERSION"
  sha256 "$MAC_SHA"
  url "$MAC_URL"
  name "PR Nanny"
  desc "Passive GitHub pull-request monitor"
  homepage "https://github.com/alexcatdad/pr-nanny"
  depends_on arch: :arm64
  depends_on macos: :ventura
  app "PR Nanny.app"
end
EOF
fi
