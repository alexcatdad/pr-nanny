# Local release signing

macOS releases are signed and notarized on the coordinator Mac by the local
release task (`task release:prepare` / `task release`; see
[CONTRIBUTING.md](../CONTRIBUTING.md#releases)). No signing or notarization
runs in GitHub Actions, and no signing credentials are held there.

## Configuration

`release.json` declares `signing.macos` for the `darwin-arm64` target with
`notarize: true` and `staple: true`. The signing identity (a Developer ID
Application certificate) and the `notarytool` keychain profile are named in the
local release-tools configuration (`~/.config/release-tools/config.json`, key
`signing.pr-nanny`). They stay in the local Keychain; Apple keys and passwords
are never committed or uploaded.

## Flow

1. The `build:darwin-arm64` task builds the app and, because the release tool
   provides `MACOS_SIGNING_IDENTITY`, `scripts/package.sh` signs the executable
   and the app bundle with the hardened runtime and a secure timestamp
   (`MACOS_ENTITLEMENTS` is passed when an entitlements file is configured; none
   is today). Without the variable, for example in CI, the app is ad-hoc signed.
2. `reltool` verifies the signature, submits the zip with `notarytool`, waits,
   staples the app ticket and re-creates the zip. The recorded SHA-256 describes
   the final bytes.
3. Interrupted notarization is resumed with `task release:resume` and never
   submitted twice.

The Windows executable and installer of earlier releases were unsigned. No new
Windows builds are produced.

Apple's [notarization guide](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
documents `notarytool` and ticket stapling.
