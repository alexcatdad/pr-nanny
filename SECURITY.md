# Security

PR Nanny is prerelease software. Report suspected vulnerabilities privately using GitHub's **Report a vulnerability** flow at https://github.com/alexcatdad/pr-nanny/security/advisories/new rather than publishing credentials or exploit details in an issue.

## Threat model

Assets are GitHub credentials, private repository metadata, local preferences, and the integrity of the desktop application. Trust boundaries are the GitHub HTTPS API, GitHub CLI, native credential store, local JSON, embedded webview, frontend-to-Go bridge, and downloaded release artifacts.

Treat repository names, PR titles, URLs, API responses, and local JSON as untrusted. A compromised renderer must not gain generic shell/filesystem access, retrieve tokens, choose a network endpoint, or open arbitrary URLs. Backend requests are queries only. PAT replacement must validate before secure storage; storage failures fail closed. CLI credentials are resolved on demand without copying into persistence.

The implementation review checks exact GitHub.com PR URL validation, escaped text rendering, restrictive CSP, bounded HTTPS requests, sanitized failures, atomic versioned JSON with private permissions, the narrow bridge, and persisted notification baselines. Local tests exercise malicious text, malformed URLs and API responses, credential leakage, symlink containment, corrupted JSON, and rejected generic runtime capabilities. Production checks inspect selected framework files, build metadata and binary contents for development/MCP capabilities; incompatible production and development tags also fail compilation.

Windows storage uses protected current-user-only DACLs; Unix storage uses 0700 directories and 0600 files. Linux native notification bodies escape XML markup independently of the webview's text rendering.

Required CI gates run race tests, native credential-store roundtrips, frontend/component/Playwright tests, dependency audits, static security analysis, actual native launch/IPC tests, and package verification. Publication requires a successful push validation on main with every platform and security job passed; release metadata comes from that exact qualified source commit. Passing these automated checks does not establish complete desktop behavior or absence of vulnerabilities.

Residual risks include compromised GitHub/CLI/native keychain or host account, Wails and system-webview vulnerabilities, GitHub permissions broader than the application's read-only behavior, visibility of PR metadata in OS notifications and local JSON, and unsigned release authenticity. CI does not prove every desktop's tray, notification, credential-store or autostart behavior. Release qualification and exact limitations are recorded separately.
