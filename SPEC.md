# PR Nanny 0.1 V1 specification

## Product boundary

PR Nanny passively monitors GitHub.com pull requests for one authenticated GitHub account. It may read GitHub API data and display local notifications. It does not mutate pull requests, reviews, checks, comments, repository settings, or GitHub review subscriptions. It does not support GitHub Enterprise Server or multiple accounts.

Supported targets are macOS 13+ ARM64, Windows 10/11 x64 with WebView2, and Linux x64 with GTK4, WebKitGTK 6, glibc 2.39+, and a Secret Service-compatible credential store. Windows distribution is a zip, not an installer. Homebrew supports macOS/Linux only, including WSL for Linux; Homebrew-based native Windows installation is unsupported, and WinGet is out of scope.

## Discovery and display

The monitor fetches open PRs authored by the account and open PRs with an individual or team review request for the account. Matching team slugs are obtained from the viewer's organizations. Underlying API data and notification transitions are deduplicated by PR ID, while the UI shows **Authored by me**, **Review requested**, and **Recent** groupings. A PR matching multiple categories appears in each matching section. Results sort by `updatedAt`, newest first.

Recent contains closed or merged PRs for 60 seconds after their actual `closedAt` timestamp. If the close time is missing, a PR cannot be presented as a recent closure. Current PRs are in memory and repopulated on refresh; there is no history archive.

Each PR row shows number, title, repository, approval state, CI summary, age since creation, and a local notification bell. PR content is untrusted text. Clicking a row opens only a validated canonical `https://github.com/{owner}/{repo}/pull/{number}` URL in the default browser. The bell changes only local PR Nanny preferences.

The popup has two locally persisted layouts: **Show all**, which groups sections in a scrollable list, and **Tabs**, which shows one category at a time. It follows the system light/dark appearance. Clicking outside or pressing Escape closes the popup. A tray menu provides Settings and Quit.

Approval is intentionally binary: **Approved** when at least one current non-author opinionated review is approved, otherwise **Not approved**. This does not express required approval counts, branch protection, or merge readiness.

CI maps the full status-check rollup to four states:

- **Passed** when all known checks are completed successfully, neutrally, or skipped.
- **Not passed** when any check has a recognized failure conclusion or commit status.
- **Running** when no failure is present and at least one check is queued or pending.
- **Not available** when no checks exist or data is missing, incomplete, malformed, or has an unknown state.

Failure takes precedence over pending; pending takes precedence over success. The app does not expose individual check details or infer mergeability.

## Authentication and settings

Exactly one account can be connected at a time. Authentication can use the currently selected GitHub CLI `gh` account or a PAT. PATs are validated before account or credential changes are committed, remain in Go, and are stored only in the native OS credential store. CLI credentials are fetched on demand and are not copied to that store. The backend never returns stored tokens to the renderer; the user-entered PAT is cleared from the input before the ConnectPAT call resolves.

Settings include authentication method, update interval, global notification toggle, event toggles, and launch at login. Intervals are 30 seconds, 60 seconds, 2 minutes, and 5 minutes. Defaults are `gh`, 60 seconds, global notifications on, all event types enabled, Show all layout, individual PR bells off, and launch at login on. Layout can also be changed in the popup. Authentication selection is a draft until Connect validates it; preference saves cannot switch accounts.

The minimum fine-grained PAT permissions are repository **Pull requests: read**, **Checks: read**, **Commit statuses: read**, and **Metadata: read**, plus organization **Members: read** for team discovery. A classic PAT needs `repo` to read private repositories and `read:org` for team discovery; `repo` is broad and cannot be reduced to read-only repository access. Organization approval and SSO authorization requirements apply according to the organization policy.

## Polling and status

The app requests an initial refresh after authentication, then polls at the selected interval. Concurrent refreshes are serialized. Offline failures back off exponentially up to five minutes; GitHub rate limits use server-provided retry delays. A failed fetch leaves the last successful rows and notification baseline intact and exposes an actionable connection status. Successful refresh time is shown in the popup. Inaccessible repositories retain their cached rows for that snapshot rather than being treated as closed.

Polling observes sampled GitHub states. Changes that start and reverse between polls cannot be detected. Pagination is handled with bounded requests; unexpected or incomplete responses fail safely rather than being interpreted as successful empty data.

## Notifications

Event preferences cover CI changes, approval changes, review requests, merges, and closures. Global notifications off suppresses every event. A new/re-requested individual or team review may notify when global notifications and the review-request event are enabled, even while that PR's bell is off. Other transitions require that PR's bell to be on and the corresponding event preference enabled. Bells are off by default.

Notification baselines are established on the first successful synchronization, so historical state does not produce a startup flood. The compact baseline and bells persist locally. The monitor persists the new baseline before asking the OS to deliver notifications; this gives at-most-once attempts across restart, but a process or OS failure after persistence can lose a notification, and OS delivery is not guaranteed. Duplicate category rows refer to one PR and one bell preference.

## Persistence and security

User settings and compact notification state use separate versioned JSON files with atomic replacement and private directory permissions. Settings include method and account metadata but never credentials. State contains PR IDs, per-PR bells, and last-observed transitions needed for deduplication. Current API data is not archived. Invalid persisted data is quarantined rather than silently deleted.

All GitHub networking and credentials remain in Go. The renderer is served from embedded local assets with a restrictive CSP and a narrow bound-service interface. GitHub values render as text. No telemetry, external application server, or database is used. See [SECURITY.md](SECURITY.md) for residual risks and reporting.

## Release qualification

Version `0.1.0-rc.1` artifacts are prepared for testing, not published. Cross-platform CI, signing, and macOS notarization are not complete. An actual Wails launch smoke test verifies the embedded UI can call its service; it is not proof of physical tray interactions on every supported desktop. Do not present a platform as fully qualified until its native CI job succeeds.
