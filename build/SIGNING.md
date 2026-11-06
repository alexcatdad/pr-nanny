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
   provides `MACOS_SIGNING_IDENTITY`, `macos/Scripts/package-app.sh` signs
   `Contents/Helpers/pr-nanny-helper` first (identifier
   `com.alexcatdad.pr-nanny.helper`) and the app bundle last, both with the
   hardened runtime and a secure timestamp. Without the variable, for example in
   CI, both are ad-hoc signed. With `MACOS_PROVISIONING_PROFILE` (below) the
   profile is embedded and the app is signed with
   `macos/Resources/PRNanny.entitlements`.
2. `reltool` verifies the signature, submits the zip with `notarytool`, waits,
   staples the app ticket and re-creates the zip. The recorded SHA-256 describes
   the final bytes.
3. Interrupted notarization is resumed with `task release:resume` and never
   submitted twice.

## GitHub token and the provisioning profile

The app, not the helper, stores the GitHub personal access token: a
generic-password item `com.alexcatdad.pr-nanny.github-token` in the
data-protection keychain, `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`, access
group `CX6D6KGCT5.com.alexcatdad.pr-nanny`. The helper gets it over its private
socket at startup and keeps it in memory only.

The data-protection keychain needs `keychain-access-groups` and
`com.apple.application-identifier`, which macOS treats as restricted: a
Developer ID app may carry them only with an embedded provisioning profile that
authorizes them. To enable it:

1. In the Apple Developer portal, give the App ID `com.alexcatdad.pr-nanny`
   (team CX6D6KGCT5) and create a **Developer ID** provisioning profile for it.
2. Save it outside the repository and export
   `MACOS_PROVISIONING_PROFILE=/path/PR_Nanny_Developer_ID.provisionprofile`
   for `task release`. `package-app.sh` checks that the profile is for
   `CX6D6KGCT5.com.alexcatdad.pr-nanny` before embedding it.

Without the profile (and in ad-hoc dev builds) SecItem reports
`errSecMissingEntitlement`, and the app stores the token in the login keychain
instead. That item's ACL trusts only the app's signature, so
`security find-generic-password` cannot read it without a prompt. Once a
release carries the profile, the app moves the item into the data-protection
keychain on its first read.

PR Nanny 0.1.x stored the token through go-keyring as `pr-nanny-github`, whose
ACL trusts `/usr/bin/security`, so any process running as the user could read
it. On first launch the app asks the helper for that item, saves the token the
new way, and has the helper delete the old item.

The Windows executable and installer of earlier releases were unsigned. No new
Windows builds are produced.

Apple's [notarization guide](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
documents `notarytool` and ticket stapling.
