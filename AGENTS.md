# PR Nanny

## Purpose

Passive GitHub.com pull-request monitor for one account: shows authored and review-requested PRs with approval and CI summaries, and raises local notifications. macOS menu-bar app (macOS 13+, Apple Silicon) with a Go backend in `cmd/pr-nanny-helper` and `internal/`. The current UI is a Wails v3 + Svelte app in the repository root and `frontend/`; a Swift app on `alexcatdad/menubar-kit` (bundle ID `com.alexcatdad.pr-nanny`) replaces it.

## Invariants

- Passive monitor: never add a GitHub mutation, subscription change, telemetry, or frontend network/auth access.
- Secrets live only in Go and the macOS Keychain (go-keyring). No plaintext fallback, no secrets in snapshots, helper replies or logs, no raw upstream error logging.
- Render API data as text. Validate GitHub PR URLs with `ValidPRURL` before opening them; the UI validates again.
- Notifications are gated in Go (global switch, event preference, bell; review requests bypass the bell). In the Swift app they are posted by the app bundle via MenubarKit: opt-in, permission requested from the user toggle, silent unless the user enables sound. Never `osascript`, `terminal-notifier` or a bare binary.
- The helper socket is 0600 in the 0700 data directory, peer-UID checked, behind the single-instance lock; the helper exits with its parent.
- Pin Wails v3.0.0-beta.33 while the Wails app exists. Production builds use `-tags production` and never include MCP or DevTools.
- Test with synthetic fixtures, never real credentials or the user's `gh` session.
- No third-party dependencies in the shipped Swift app without a recorded decision. First-party `alexcatdad/menubar-kit` is allowed, pinned to an exact revision.

## Layout

- `cmd/pr-nanny-helper`, `internal/helper` — backend process and its socket API ([docs/HELPER-API.md](docs/HELPER-API.md)).
- `internal/{app,core,github,auth,storage,notify,model}` — domain logic, no UI imports.
- Root `*.go`, `frontend/`, `internal/websecurity` — Wails adapters and Svelte UI (being replaced).
- `scripts/`, `build/` — packaging and verification. `site/` — product page (Pages). `docs/` — runbooks and evidence. User-facing product truth lives in `README.md`.

## Build & verify

- `task check` — the full local gate: lint, build, test, test:integration, security. CI runs the same tasks.
- `task lint`, `task build`, `task test`, `task test:integration`, `task security` — what CI calls (`alexcatdad/ci-actions` validate-pr/validate-main).
- Toolchain: Go 1.27.3, Task 3, Node 24 + pnpm 11 (corepack) for `frontend/` and `site/`; Swift 6 on Xcode for macOS 26 once `macos/` exists.

## Release

- Local only, with release-tools (`task release:check`, `task release:prepare`, `task release`). Developer ID signed, notarized and stapled; keys stay in the local Keychain.
- Versions are CalVer `vYYYY.MM.DD.N`, allocated by release-tools; do not edit `VERSION` for a release. Releases are immutable: never replace published assets or move tags.
- Distribution: Homebrew cask in `alexcatdad/homebrew-tap`. Never strip quarantine.

## Git

- Canonical host: GitHub `alexcatdad/pr-nanny`.
- Branch from `main`, PR back to `main`, squash-merge. Conventional commit messages.
- Required checks: `pr / leaks`, `pr / lint`, `pr / build`, `pr / deps`, `codeql`. Copilot reviews every PR.
- Actions are pinned to full commit SHAs; workflows start from `permissions: {}`.
- Do not wait on or poll CI; one status check when a decision depends on it. Respect existing work and coordinate file ownership among agents.

## Memory

Decisions, constraints and project state live in Scratchpad (project `pr-nanny`), not in repo files. Read context there before substantial work and record meaningful decisions there.
