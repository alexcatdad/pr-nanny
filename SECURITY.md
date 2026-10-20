# Security

PR Nanny is prerelease software. Report suspected vulnerabilities privately using GitHub's **Report a vulnerability** flow at https://github.com/alexcatdad/pr-nanny/security/advisories/new rather than publishing credentials or exploit details in an issue.

## Threat model

Assets are GitHub credentials, private repository metadata, local preferences, and the integrity of the desktop application. Trust boundaries are the GitHub HTTPS API, GitHub CLI, native credential store, local JSON, embedded webview, frontend-to-Go bridge, and downloaded release artifacts.

Treat repository names, PR titles, URLs, API responses, and local JSON as untrusted. The renderer bridge must not expose generic shell/filesystem access, stored tokens, a configurable backend network endpoint, or arbitrary browser opening. Backend requests are queries only. PAT replacement must validate before secure storage; storage failures fail closed. CLI credentials are resolved on demand without copying into persistence.

The implementation review checks exact GitHub.com PR URL validation, escaped text rendering, restrictive CSP, bounded HTTPS requests, sanitized failures, atomic versioned JSON with private permissions, the narrow bridge, and persisted notification baselines. Local tests exercise malicious text, malformed URLs and API responses, credential leakage, symlink containment, corrupted JSON, and rejected generic runtime capabilities. Production checks inspect selected framework files, build metadata and binary contents for development/MCP capabilities; incompatible production and development tags also fail compilation.

Windows storage uses protected current-user-only DACLs; Unix storage uses 0700 directories and 0600 files. Linux native notification bodies escape XML markup independently of the webview's text rendering.

Required CI gates run race tests, native credential-store roundtrips, frontend/component/Playwright tests, dependency audits, static security analysis, actual native launch/IPC tests, and package verification. Publication requires a successful push validation on main with every platform and security job passed; release metadata comes from that exact qualified source commit. Passing these automated checks does not establish complete desktop behavior or absence of vulnerabilities.

The quality toolchain also scans source history with Gitleaks, workflows with zizmor, and Go/TypeScript with CodeQL. Gitleaks excludes one exact historical fingerprint for a synthetic test token; no secret detector or application path is disabled. Native main builds generate artifact attestations, and publication verifies the archive digests, source commit and qualification workflow identity. Release tags reject updates/deletion and new releases use GitHub's immutable-release protection. These provenance controls establish origin and qualification, not Developer ID/Authenticode signing.

## Renderer containment limit

CSP restricts script, resource and connection requests; middleware denies generic Wails runtime operations, including arbitrary browser/window IPC. These controls do **not** fully contain JavaScript that has already gained execution in the renderer. An isolated macOS production build of pinned Wails beta.33 confirmed that `window.location` can navigate the webview to an arbitrary URL despite CSP. That could disclose PR metadata available to a compromised renderer. No supported cross-platform navigation-veto API was found in this framework version, and this review found no PR-title or repository-name path to script execution. Keep API values rendered as text and prohibit remote code. Do not treat CSP as a complete post-compromise network sandbox.

Residual risks include compromised GitHub/CLI/native keychain or host account, Wails and system-webview vulnerabilities, GitHub permissions broader than the application's read-only behavior, visibility of PR metadata in OS notifications and local JSON, and unsigned release authenticity. CI does not prove every desktop's tray, notification, credential-store or autostart behavior. Release qualification and exact limitations are recorded separately.
