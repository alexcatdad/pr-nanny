# Release packaging

`VERSION` is the SemVer source for development and CI builds. When `RELEASE_VERSION` is set (local releases), `scripts/read-version.sh` and the frontend build use it instead. `scripts/package.sh [VERSION] darwin` creates the macOS ARM64 app zip. An optional version must match the resolved version. The script passes that value to the Go binary and the frontend build resolves the same value. Packaging uses the `production` build tag and writes a SHA-256 sidecar next to the archive. Set `PR_NANNY_PREBUILT_FRONTEND=1` only when `frontend/dist/index.html` was built from this same source tree. Run `scripts/verify-package.sh` after producing archives.

Release archives are produced by the local release task (`task release`; see [CONTRIBUTING.md](../CONTRIBUTING.md#releases)), which passes the reserved version as `RELEASE_VERSION`. The macOS app is Developer ID signed, notarised and stapled locally (see [SIGNING.md](SIGNING.md)). macOS ARM64 is the only target; Windows and Linux assets of earlier releases remain as history.

The tap (`alexcatdad/homebrew-tap`) cask `Casks/pr-nanny.rb` is updated by `task release` from the published, verified release. Install with:

```sh
brew install --cask alexcatdad/tap/pr-nanny
```

The cask does not remove macOS quarantine or bypass Gatekeeper. The Linux `Formula/pr-nanny.rb` is no longer updated.
