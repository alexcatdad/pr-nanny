# PR Nanny

[![CI](https://github.com/alexcatdad/pr-nanny/actions/workflows/ci.yml/badge.svg)](https://github.com/alexcatdad/pr-nanny/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/alexcatdad/pr-nanny?include_prereleases)](https://github.com/alexcatdad/pr-nanny/releases) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

PR Nanny is a passive GitHub.com pull-request monitor that lives in the system tray. It reads open pull requests authored by you or requesting your individual or team review, shows their approval and CI summaries, and can send local notifications. It never writes to GitHub or changes subscriptions.

> **PR Nanny is beta software.** See [Releases](https://github.com/alexcatdad/pr-nanny/releases) for published downloads and [Actions](https://github.com/alexcatdad/pr-nanny/actions/workflows/ci.yml) for exact-source qualification. Published macOS distributions use Developer ID signing and Apple notarisation. Windows distributions are unsigned.

## Screenshots

The screenshots use synthetic pull-request fixtures; no GitHub account data is shown.

| Popup: all sections | Popup: tabs |
| --- | --- |
| ![All sections, light appearance](frontend/screenshots/popup-light-all.png) | ![Tabs, light appearance](frontend/screenshots/popup-light-tabs.png) |
| ![All sections, dark appearance](frontend/screenshots/popup-dark-all.png) | ![Tabs, dark appearance](frontend/screenshots/popup-dark-tabs.png) |

| Settings: light appearance | Settings: dark appearance |
| --- | --- |
| ![Settings in light appearance](frontend/screenshots/settings-light.png) | ![Settings in dark appearance](frontend/screenshots/settings-dark.png) |

## What it does

- Offers **Show all** grouped sections and **Tabs** layouts, sorted by GitHub's most recently updated time. Remembers the layout locally.
- Displays approval as **Approved** or **Not approved**, and CI as **Passed**, **Not passed**, **Running**, or **Not available**. Unknown or incomplete CI data is never shown as a failure.
- Keeps merged or closed PRs in **Recent** for 60 seconds from the actual closure time when GitHub supplies it.
- Polls every 60 seconds by default and supports 30 seconds, 2 minutes, or 5 minutes. Offline and rate-limit states preserve the last fetched rows.
- Supports existing `gh` CLI authentication or a validated PAT held in the operating system credential store. The frontend never handles stored credentials.
- Sends review-request notifications when global notifications and that event are enabled. Other change notifications require that PR's bell to be enabled; bells start off.

CI summaries combine check runs and commit statuses. A failure takes precedence over a pending check; pending checks show Running; all successful, neutral, or skipped checks show Passed. Missing, incomplete, malformed, or unknown results show Not available. Approval means at least one current non-author opinionated review is approved; it is not a merge-readiness calculation.

## Download and install

Qualified release archives use the version from [`VERSION`](VERSION):

- **macOS:** `pr-nanny-VERSION-darwin-arm64.dmg` — macOS 13 or newer, Apple Silicon. Open the DMG and drag **PR Nanny.app** onto **Applications**.
- **Windows:** `pr-nanny-VERSION-windows-x64-setup.exe` — Windows 10/11 x64. Run Setup; it installs for your account, adds a Start Menu entry and supplies Microsoft WebView2 when missing. Removing the app through Windows Settings preserves preferences and credentials.
- Portable `.zip` archives remain available for macOS and Windows.
- `pr-nanny-VERSION-linux-x64.tar.gz` — Linux x64 with glibc 2.39 or newer, GTK4, WebKitGTK 6, and a Secret Service-compatible credential store such as GNOME Keyring or KWallet with Secret Service enabled. Ubuntu 24.04 and current Fedora releases are the target environments.

Download qualified archives from [Releases](https://github.com/alexcatdad/pr-nanny/releases). Before a release qualifies, each successful native CI job provides a `package-*` workflow artifact for testing. Verify the adjacent `.sha256` file or release `SHA256SUMS` before installing. A successful individual platform job does not establish qualification of the other platforms.

The macOS app and DMG are Developer ID signed, notarised and stapled before publication. Signing credentials stay in the maintainer's local Keychain. The ZIP remains available for Homebrew and portable installation.

The Windows installer and executable are unsigned and may show a publisher or SmartScreen warning. The installer requires an internet connection only if it needs to install WebView2. The optional portable ZIP includes `install.ps1` and `uninstall.ps1` for shortcut management.

On Linux, extract the archive, run `./install.sh` from its extracted directory, and ensure `~/.local/bin` is on `PATH`. The installer adds the app and desktop entry for the current user. Run `./uninstall.sh` in the same directory to remove those application files.

Install scripts preserve settings and credentials. The existing [`alexcatdad/tap`](https://github.com/alexcatdad/homebrew-tap) is the Homebrew distribution target; see [Homebrew installation notes and integration status](build/README.md) before installing. Homebrew on Windows runs Linux packages inside WSL; the native Windows archive remains a separate installation. [Generated release notes](https://github.com/alexcatdad/pr-nanny/releases) are the changelog.

## Authentication and permissions

The first launch stays in the tray. Choose **GitHub CLI** to reuse the existing `gh` session, or provide a PAT in Settings. PR Nanny validates a PAT before switching accounts and stores it only in macOS Keychain, Windows Credential Manager, or Linux Secret Service. A credential-store failure does not use a plaintext fallback. `gh` tokens are fetched when needed and are not copied into PR Nanny's credential store.

For a **fine-grained PAT**, grant repository **Pull requests: read**, **Checks: read**, **Commit statuses: read**, and **Metadata: read**; grant organization **Members: read** to discover your teams. Limit repository access to the repositories you want to monitor. Organization policy may require owner approval before a token can read private resources. For a **classic PAT**, private repository access requires the broad `repo` scope, and team discovery requires `read:org`; classic scopes cannot be narrowed to read-only repository permissions. Authorize the classic token for each organization that enforces SAML SSO. See GitHub's [fine-grained token permission reference](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens) and [SSO authorization guidance](https://docs.github.com/authentication/authenticating-with-saml-single-sign-on/authorizing-a-personal-access-token-for-use-with-single-sign-on).

## Build from source

Use Go 1.27.2, Node.js 24.19.0, pnpm 11.25.0, and the pinned Wails `v3.0.0-beta.33` dependency. See [CONTRIBUTING.md](CONTRIBUTING.md) for platform prerequisites, tests, and production build commands. Production packages use `-tags production`; do not use development tooling with production builds.

Settings and notification deduplication state are stored as versioned JSON under the OS user config directory; current PR data remains in memory. See [SPEC.md](SPEC.md), [ARCHITECTURE.md](ARCHITECTURE.md), and [SECURITY.md](SECURITY.md) for behavior, data flow, and known limitations.

Ubuntu 24.04+ may need a per-application AppArmor allowance so WebKit can create its sandbox. See [the exact-path profile instructions](build/linux/APPARMOR.md); keep WebKit sandboxing and the system-wide policy enabled.

## Development and verification

Run `sh scripts/lint.sh` for local formatting, lint and type checks; see [CONTRIBUTING.md](CONTRIBUTING.md) for tests and platform prerequisites. CI retains readable Go and frontend coverage artifacts. Release publication requires every qualification job on the exact source commit, matching package checksums, and verified build provenance. Review [the implementation checkpoint](docs/IMPLEMENTATION-STATUS.md) for the distinction between completed and pending qualification.
