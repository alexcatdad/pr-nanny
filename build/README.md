# Release packaging

`VERSION` is the SemVer source for development and CI builds. When `RELEASE_VERSION` is set (local releases), `scripts/read-version.sh` and the frontend build use it instead. `scripts/package.sh [VERSION] darwin` or
`scripts/package.sh [VERSION] linux` creates macOS or Linux archives; Windows
uses `scripts/package-windows.ps1 [-Version VERSION]`. An optional version must
match the resolved version. Each script passes that value to the Go binary and the
frontend build resolves the same value. Packaging uses the
`production` build tag and writes a SHA-256 sidecar next to each archive. Set
`PR_NANNY_PREBUILT_FRONTEND=1` only when `frontend/dist/index.html` was built
from this same source tree. Run `scripts/verify-package.sh` after producing
archives.

Release archives are produced by the local release task (`task release`; see [CONTRIBUTING.md](../CONTRIBUTING.md#releases)), which passes the reserved version as `RELEASE_VERSION`. The macOS app is Developer ID signed, notarised and stapled locally (see [SIGNING.md](SIGNING.md)); CI archives are ad-hoc-signed validation builds. The release matrix is macOS ARM64 (zip) and Linux x64 (tar.gz). Windows packaging scripts remain for CI validation, but no Windows release is built or published; earlier Windows assets remain as history.
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

Homebrew supports macOS and Linux (including WSL); it has no native Windows
package support. The tap (`alexcatdad/homebrew-tap`) is updated by the local
release task from the published, verified release: `Casks/pr-nanny.rb` (macOS
arm64 zip) is updated by `task release`, and `Formula/pr-nanny.rb` (Linux x64
tar.gz) is updated by the same task as a separate step.
The Linux formula relies on system GTK4 and WebKitGTK 6.0.

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
