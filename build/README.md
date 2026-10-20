# Release packaging

`VERSION` is the single SemVer source for application builds and release
metadata. `scripts/package.sh [VERSION] darwin` or
`scripts/package.sh [VERSION] linux` creates macOS or Linux archives; Windows
uses `scripts/package-windows.ps1 [-Version VERSION]`. An optional version must
match `VERSION`. Each script passes that value to the Go binary and the
frontend build reads the same root `VERSION` file. Packaging uses the
`production` build tag and writes a SHA-256 sidecar next to each archive. Set
`PR_NANNY_PREBUILT_FRONTEND=1` only when `frontend/dist/index.html` was built
from this same source tree. Run `scripts/verify-package.sh` after producing
archives.

The macOS ARM64 archive is ad-hoc signed for bundle integrity only. It has no
Developer ID signature or notarization, so Gatekeeper may require the user to
approve opening the prerelease. Windows is unsigned. Linux requires GTK4,
WebKitGTK 6.0, glibc 2.39 or newer, and a Secret Service-compatible credential
store. Linux binaries are built on Ubuntu 24.04. Its installer and uninstaller
only add or remove files under the current user's XDG data and bin directories;
settings and credentials remain untouched.

Homebrew supports macOS and Linux (including WSL); it has no native Windows
package support. `scripts/homebrew.sh cask VERSION URL SHA256` and
`scripts/homebrew.sh formula VERSION URL SHA256` emit separate templates. Both
commands reject a version that does not match `VERSION` and require real HTTPS
archive URLs and SHA-256 values. The Linux formula relies on system GTK4 and
WebKitGTK 6.0. Qualified GitHub Releases include `pr-nanny-cask.rb` and
`pr-nanny-formula.rb`; the release workflow verifies each archive's source
attestation and checksum before publishing them. Tap changes are reviewed
separately in the existing `alexcatdad/homebrew-tap` repository.

Once the PR Nanny entries are present in the tap, install with:

```sh
brew tap alexcatdad/tap
brew install --cask alexcatdad/tap/pr-nanny # macOS ARM64
brew install alexcatdad/tap/pr-nanny       # Linux x64
```

The cask does not remove macOS quarantine or bypass Gatekeeper. The Linux
formula requires GTK4, WebKitGTK 6.0, a Secret Service provider and glibc 2.39+
and does not make older distributions compatible with the Ubuntu 24.04 binary
baseline.
