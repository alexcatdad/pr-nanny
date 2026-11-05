# Release packaging

`VERSION` is the SemVer source for development and CI builds. When `RELEASE_VERSION` is set (local releases), `scripts/read-version.sh` uses it instead. `scripts/package.sh [VERSION] darwin` creates the macOS ARM64 app zip through `macos/Scripts/package-app.sh`: the Swift app in `Contents/MacOS/PRNanny`, the production helper in `Contents/Helpers/pr-nanny-helper` (reports the version with `-version`), `build/darwin/icon.icns` as the app icon, and the tray glyph. The helper is signed first and the app last. An optional version must match the resolved version; `MACOS_SHORT_VERSION`/`MACOS_BUNDLE_VERSION` set the Info.plist versions. A SHA-256 sidecar is written next to the archive. Run `scripts/verify-package.sh` after producing archives.

Release archives are produced by the local release task (`task release`; see [CONTRIBUTING.md](../CONTRIBUTING.md#releases)), which passes the reserved version as `RELEASE_VERSION`. The macOS app is Developer ID signed, notarised and stapled locally (see [SIGNING.md](SIGNING.md)). macOS ARM64 is the only target; Windows and Linux assets of earlier releases remain as history.

The tap (`alexcatdad/homebrew-tap`) cask `Casks/pr-nanny.rb` is updated by `task release` from the published, verified release. Install with:

```sh
brew install --cask alexcatdad/tap/pr-nanny
```

The cask does not remove macOS quarantine or bypass Gatekeeper. The Linux `Formula/pr-nanny.rb` is no longer updated.
