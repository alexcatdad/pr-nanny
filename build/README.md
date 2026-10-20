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

CI's macOS ARM64 archive is an ad-hoc-signed input to local release signing.
Published macOS betas are Developer ID signed and notarised; the DMG provides
drag-to-Applications installation and the ZIP remains the Homebrew input.
Windows also produces a per-user `*-setup.exe` with a Start Menu shortcut,
uninstaller and Microsoft-signed WebView2 bootstrapper. Windows remains unsigned.
Linux requires GTK4,
WebKitGTK 6.0, glibc 2.39 or newer, and a Secret Service-compatible credential
store. Linux binaries are built on Ubuntu 24.04. Its installer and uninstaller
only add or remove files under the current user's XDG data and bin directories;
settings and credentials remain untouched.

Windows setup acceptance runs on the native Windows CI runner: install twice
into a path containing spaces, verify the installed executable and shortcut,
launch the isolated frontend/IPC smoke test, then uninstall and verify that
synthetic user data survives. This runner already has WebView2; the missing-runtime
bootstrapper path still requires separate acceptance.

After signing and stapling the macOS app, create the DMG with
`CODESIGN_IDENTITY='Developer ID Application: ...' sh scripts/package-dmg.sh SIGNED.zip VERSION dist`.
Submit the DMG using the existing local notarytool Keychain profile, staple the
accepted DMG, then run `sh scripts/verify-dmg.sh IMAGE.dmg VERSION SIGNED.zip`.
Only compute its checksum after stapling. Verification checks the image and app
signatures/tickets, the Applications link and the exact ZIP-contained app.

Homebrew supports macOS and Linux (including WSL); it has no native Windows
package support. `scripts/homebrew.sh cask VERSION URL SHA256` and
`scripts/homebrew.sh formula VERSION URL SHA256` emit separate templates. Both
commands reject a version that does not match `VERSION` and require real HTTPS
archive URLs and SHA-256 values. The Linux formula relies on system GTK4 and
WebKitGTK 6.0. Qualified GitHub Releases include `pr-nanny-cask.rb` and
`pr-nanny-formula.rb`; the release workflow verifies each archive's source
attestation and checksum before publishing them. Tap changes are reviewed
separately in the existing `alexcatdad/homebrew-tap` repository.

PR Nanny is available in the existing tap. Install with:

```sh
brew tap alexcatdad/tap
brew install --cask alexcatdad/tap/pr-nanny # macOS ARM64
brew install alexcatdad/tap/pr-nanny       # Linux x64
```

The cask does not remove macOS quarantine or bypass Gatekeeper. The Linux
formula requires GTK4, WebKitGTK 6.0, a Secret Service provider and glibc 2.39+
and does not make older distributions compatible with the Ubuntu 24.04 binary
baseline.

The existing tap has no native Windows installer mechanism. Homebrew's supported
Windows environment is [WSL 2](https://docs.brew.sh/Homebrew-on-Linux), where this
formula installs the Linux build. GUI use also requires WSLg or another desktop
session and the Linux runtime/vault dependencies. Use the Windows ZIP for the
native Windows application.

After publishing a new release, the source workflow dispatches the tap's
verified release importer with a short-lived GitHub App installation token.
Configure the existing app's `APP_ID` variable and `APP_SECRET` private key
secret in the `action-runners` environment. The token is limited to
`alexcatdad/homebrew-tap` with Actions write access. If either value is missing,
the release succeeds and the workflow reports that the tap update was skipped;
maintainers can manually run `update-pr-nanny.yml` in the tap with the qualified
release tag. The importer verifies all three source-bound attestations and
checksums and opens a reviewed update PR. App-dispatched PRs explicitly run the
tap checks. The narrowly pinned RC1 bootstrap is documented in the tap and does
not claim attestations for RC1.
