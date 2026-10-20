# Local release signing

Published macOS releases are signed on the maintainer's Mac. The existing
Developer ID identity and `usb-boop-notary` credential profile stay in the local
Keychain; Apple keys and passwords are not uploaded to GitHub. RC6 completed
this process with Apple acceptance, a stapled app ticket and Gatekeeper acceptance.

For installer betas, sign and notarise the exact qualified macOS CI archive,
staple its app, and recreate the ZIP. Then use `scripts/package-dmg.sh` to wrap
that unchanged app with an Applications link. Sign, notarise and staple the DMG
itself, and verify it against the ZIP with `scripts/verify-dmg.sh` before creating
the final checksum. Do not reuse a prior submission for a changed artifact.

The Windows Setup EXE is built and install/reinstall/uninstall-tested by native
CI. It remains unsigned until an Authenticode identity is supplied.

## Optional CI signing alternative

`.github/workflows/sign.yml` is a manual, optional step for signing qualified `0.1.0-rc.1` packages. It does not build or change the source package. It accepts only an Actions run whose API metadata identifies the `Validate` workflow at `.github/workflows/ci.yml`, a successful `push` to `main`, and a full 40-character source commit SHA. Each signing job checks out that exact SHA and downloads its platform package from that same run using the read-only Actions token.

Run **Sign qualified packages** from the `main` branch and enter the successful qualification run ID. The selected run must have finished successfully, including its native package, smoke, and validation jobs. Pull-request runs, manually dispatched validation runs, other branches, incomplete runs, and failed runs are rejected before any signing job starts. The dispatch itself does not publish a release.

## Optional repository secrets

Configure each platform's complete set of GitHub Actions repository or environment secrets. Do not commit certificate files, passwords, Apple credentials, or encoded secrets.

For macOS signing and notarization:

- `MACOS_CERTIFICATE_P12_BASE64` — base64-encoded exportable Developer ID Application identity in PKCS#12 format, including its private key.
- `MACOS_CERTIFICATE_PASSWORD` — password protecting that PKCS#12 file.
- `MACOS_SIGNING_IDENTITY` — exact certificate identity string visible to `codesign`.
- `APPLE_ID` — Apple Account used for notarization.
- `APPLE_TEAM_ID` — Apple Developer team identifier associated with the signing identity.
- `APPLE_APP_PASSWORD` — Apple app-specific password for `notarytool` authentication.

The job creates a random-password keychain on the ephemeral macOS runner, imports the certificate there, uses hardened runtime and secure timestamps, verifies the signature, submits the app archive to Apple's notary service with a 20-minute wait limit, staples and validates the ticket, checks Gatekeeper assessment, then creates a new zip and SHA-256 sidecar. Its trap restores the runner's keychain search list and deletes the temporary keychain, certificate file, and working tree.

For Windows Authenticode signing:

- `WINDOWS_CERTIFICATE_PFX_BASE64` — base64-encoded exportable code-signing PFX containing the certificate and private key.
- `WINDOWS_CERTIFICATE_PASSWORD` — password protecting that PFX.

The Windows job signs the executable with SHA-256 and an RFC 3161 timestamp, verifies it with SignTool's Authenticode policy, then creates a new zip and SHA-256 sidecar. The PFX file and extracted working directory are deleted in a cleanup block. The workflow expects a password-exportable PFX; non-exportable, hardware-backed, cloud-provider, or managed signing keys are not supported by this job and require a separate integration.

If a platform's secrets are missing, the preflight emits an Actions notice and that platform job is skipped without failing the other configured platform. No credentials are required for unsigned validation builds. Signing jobs have only `actions: read` and `contents: read` permissions. They upload signed packages as 14-day Actions artifacts named `signed-package-macOS-ARM64` and `signed-package-Windows-X64`; they never create or publish GitHub Releases.

## CI signing status

The optional CI signing workflow has not been executed; its Apple and Windows signing secrets have not been supplied. Local macOS signing is independently verified. Passing YAML validation or an unsigned CI run does not establish CI certificate import, notarisation, Gatekeeper or Authenticode acceptance.

Apple's [notarization guide](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution) documents `notarytool` and ticket stapling. Microsoft's [SignTool reference](https://learn.microsoft.com/en-us/windows/win32/seccrypto/signtool) documents the SHA-256 file and timestamp digest requirements and signature verification commands.
