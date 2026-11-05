# Security

PR Nanny is prerelease software. Report suspected vulnerabilities privately using GitHub's **Report a vulnerability** flow at https://github.com/alexcatdad/pr-nanny/security/advisories/new rather than publishing credentials or exploit details in an issue.

## Threat model

Assets are GitHub credentials, private repository metadata, local preferences, and the integrity of the desktop application. Trust boundaries are the GitHub HTTPS API, GitHub CLI, macOS Keychain, local JSON, the app-to-helper unix socket, macOS notifications, and downloaded release artifacts.

Treat repository names, PR titles, URLs, API responses, and local JSON as untrusted. The helper socket must not expose generic shell/filesystem access, stored tokens, a configurable backend network endpoint, or arbitrary URL opening. Backend requests are queries only. PAT replacement must validate before secure storage; storage failures fail closed. CLI credentials are resolved on demand without copying into persistence.

The implementation review checks exact GitHub.com PR URL validation (in Go and again in Swift before `NSWorkspace.open`), plain-text rendering (`Text(verbatim:)`), bounded HTTPS requests, sanitized failures, atomic versioned JSON with private permissions, the narrow socket API, and persisted notification baselines. Local tests exercise malicious text, malformed URLs and API responses, credential leakage, symlink containment, corrupted JSON and malformed helper replies. Production checks inspect the helper's build metadata and contents for development tags, Wails and fixtures.

Storage uses a 0700 directory and 0600 files. One process owns that directory at a time through an exclusive `pr-nanny.lock` flock. The `pr-nanny-helper` socket is created 0600 inside it, and the helper rejects peers whose UID differs from its own. Helper replies never contain tokens, and errors carry fixed messages only.

Required CI gates (`pr / leaks`, `pr / lint`, `pr / build`, `pr / deps`, `codeql`) run through `alexcatdad/ci-actions`; main runs race tests, Swift tests, the bundle smoke test, gitleaks, govulncheck and gosec. Releases are local only (see CONTRIBUTING.md). Passing these automated checks does not establish complete desktop behavior or absence of vulnerabilities.

The quality toolchain also scans source history with Gitleaks, workflows with zizmor, and Go, Swift and site JavaScript/TypeScript with CodeQL. Gitleaks excludes one exact historical fingerprint for a synthetic test token; no secret detector or application path is disabled. Native main builds generate artifact attestations, and publication verifies the archive digests, source commit and qualification workflow identity. Release tags reject updates/deletion and new releases use GitHub's immutable-release protection. These provenance controls establish origin and qualification, not Developer ID/Authenticode signing.

## Native UI

The UI is native SwiftUI with no webview, so the former renderer containment limit no longer applies. Notifications are posted by the signed app bundle through `UNUserNotificationCenter` (MenubarKit), opt-in and silent by default; never through `osascript` or a shell. Notification text (repository, number, PR title) is visible in Notification Center.

Residual risks include compromised GitHub/CLI/Keychain or host account, GitHub permissions broader than the application's read-only behavior, visibility of PR metadata in OS notifications and local JSON, and unsigned release authenticity. CI does not prove every desktop's tray, notification, credential-store or autostart behavior. Release qualification and exact limitations are recorded separately.
