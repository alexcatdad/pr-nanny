# PR Nanny

## Purpose

Passive GitHub.com pull-request monitor for one account: shows authored and review-requested PRs with approval and CI summaries, and raises local notifications. Native Swift menu-bar app (macOS 13+, Apple Silicon; bundle ID `com.alexcatdad.pr-nanny`, `.dev` for development) on `alexcatdad/menubar-kit`, supervising a Go backend in `cmd/pr-nanny-helper` and `internal/`.

## Invariants

- Passive monitor: never add a GitHub mutation, subscription change, telemetry, or network/auth access from the Swift app (it talks only to the helper socket).
- The app owns the PAT in its keychain item (`TokenStore`, data-protection keychain when the profile is embedded); the helper holds it in memory only and uses go-keyring solely to migrate and delete the 0.1.x item. No plaintext fallback, no secrets in snapshots, helper replies or logs, no raw upstream error logging.
- Render API data as text (`Text(verbatim:)`). Validate GitHub PR URLs with `ValidPRURL` before opening them; the app validates again with `PRURL.isValid`.
- The app reads cached snapshots only; only the user's Refresh action calls `refresh`. Respect the polling budget in `docs/API-COST.md`.
- Notifications are gated in Go (global switch, event preference, bell; review requests bypass the bell). In the Swift app they are posted by the app bundle via MenubarKit: opt-in, permission requested from the user toggle, silent unless the user enables sound. Never `osascript`, `terminal-notifier` or a bare binary.
- The helper socket is 0600 in the 0700 data directory, peer-UID checked, behind the single-instance lock; the helper exits with its parent.
- The bundled helper is built with `-tags production`; never add development tags, Wails or fixtures to it (`scripts/check-production.sh`).
- Test with synthetic fixtures, never real credentials or the user's `gh` session.
- No third-party dependencies in the shipped Swift app without a recorded decision. First-party `alexcatdad/menubar-kit` is allowed, pinned to an exact revision.

## Layout

- `cmd/pr-nanny-helper`, `internal/helper` — backend process and its socket API ([docs/HELPER-API.md](docs/HELPER-API.md)).
- `internal/{app,core,github,auth,storage,notify,model}` — domain logic, no UI imports.
- `macos/` — Swift package: `PRNannyNative` (testable logic) and the thin `PRNanny` app target; `macos/Scripts` builds, validates and signs the bundle (helper first, app last).
- `scripts/`, `build/` — packaging and verification. `site/` — product page (Pages). `docs/` — runbooks and evidence. User-facing product truth lives in `README.md`.

## Build & verify

- `task check` — the full local gate: lint, build, test, test:integration, security. CI runs the same tasks.
- `task lint`, `task build`, `task test`, `task test:integration`, `task security` — what CI calls (`alexcatdad/ci-actions` validate-pr/validate-main).
- `task dev:run` — build and launch `PR Nanny Dev.app` (own data directory `pr-nanny-dev`).
- Toolchain: Go 1.27.3, Task 3, Swift 6 on Xcode (macOS 26); Node 24 + pnpm 11 only for `site/`.

## Release

- Local only, with release-tools (`task release:check`, `task release:prepare`, `task release`). Developer ID signed, notarized and stapled; keys stay in the local Keychain.
- Versions are CalVer `vYYYY.MM.DD.N`, allocated by release-tools; do not edit `VERSION` for a release. Releases are immutable: never replace published assets or move tags.
- Distribution: Homebrew cask in `alexcatdad/homebrew-tap`. Never strip quarantine.

## Git

- Canonical host: GitHub `alexcatdad/pr-nanny`.
- Branch from `main`, PR back to `main`, squash-merge. Conventional commit messages.
- Required checks: `pr / leaks`, `pr / lint`, `pr / build`, `pr / deps`, plus the code-scanning rule (`codeql / <lang>` checks). Copilot reviews every PR.
- Actions are pinned to full commit SHAs; workflows start from `permissions: {}`.
- Do not wait on or poll CI; one status check when a decision depends on it. Respect existing work and coordinate file ownership among agents.

## Memory

Decisions, constraints and project state live in Scratchpad (project `pr-nanny`), not in repo files. Read context there before substantial work and record meaningful decisions there.
