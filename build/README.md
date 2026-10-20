# Release packaging

Release archives are created locally with `scripts/package.sh VERSION darwin` or
`scripts/package.sh VERSION linux`, and on Windows with
`scripts/package-windows.ps1 -Version VERSION`. The scripts build the embedded
frontend and a Go binary using the `production` build tag, then write a SHA-256
sidecar next to each archive. Run `scripts/verify-package.sh` after producing
archives.

The macOS ARM64 archive is ad-hoc signed for bundle integrity only. It has no
Developer ID signature or notarization, so Gatekeeper may require the user to
approve opening the prerelease. Windows is unsigned. Linux requires GTK4,
WebKitGTK 6.0, glibc 2.39 or newer, and a Secret Service-compatible credential
store. Linux binaries are built on Ubuntu 24.04. Its installer and uninstaller
only add or remove files under the current user's XDG data and bin directories;
settings and credentials remain untouched.

Homebrew supports macOS and Linux (and Linux on WSL); it has no native Windows
package support. `scripts/homebrew.sh cask VERSION URL SHA256` and
`scripts/homebrew.sh formula VERSION URL SHA256` emit separate templates; the
`both` mode takes both archive URLs and hashes. Templates require real HTTPS
archive URLs and SHA-256 values. The Linux formula relies on the distribution's
GTK4 and WebKitGTK 6.0 runtime libraries rather than Homebrew-provided GTK
libraries. The generator does not publish a tap or provide placeholder hashes.

Qualified GitHub Releases include `pr-nanny-cask.rb` and
`pr-nanny-formula.rb` with actual release URLs and checksums. To install one
through a local tap, download the appropriate file from the release, then:

```sh
brew tap-new --no-git local/pr-nanny
tap_dir=$(brew --repository local/pr-nanny)
# macOS ARM64:
mkdir -p "$tap_dir/Casks"
cp pr-nanny-cask.rb "$tap_dir/Casks/pr-nanny.rb"
brew install --cask local/pr-nanny/pr-nanny
# Linux x64 instead:
mkdir -p "$tap_dir/Formula"
cp pr-nanny-formula.rb "$tap_dir/Formula/pr-nanny.rb"
brew install local/pr-nanny/pr-nanny
```

The cask does not remove macOS quarantine or bypass Gatekeeper. The Linux
formula requires the system libraries listed above and does not make an old
distribution compatible with the Ubuntu 24.04 binary baseline.
