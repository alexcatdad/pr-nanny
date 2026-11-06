# Changelog

All notable changes are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are CalVer
`vYYYY.MM.DD.N`. Earlier releases are described in their
[GitHub release notes](https://github.com/alexcatdad/pr-nanny/releases).

## [Unreleased]

### Added

- Stricter gates: golangci-lint adds gosec, errorlint, bodyclose, misspell and gocritic diagnostics; Swift tests build with warnings as errors and `task test` enforces a 69% `PRNannyNative` line-coverage floor.
- Native Swift menu-bar app (`macos/`) on [menubar-kit](https://github.com/alexcatdad/menubar-kit): template tray glyph with an attention badge, status-aware VoiceOver label, 340pt panel with Show all/Tabs layouts and per-PR bells, and a Settings window. It supervises the bundled `pr-nanny-helper`.
- `pr-nanny-helper`: the backend as a separate process with a versioned JSON API over a private unix socket, for the coming native menu-bar app ([docs/HELPER-API.md](docs/HELPER-API.md)).
- A single-instance lock on the data directory, shared by the app and the helper.

### Changed

- **Bundle ID is now `com.alexcatdad.pr-nanny`** (was `io.github.alexcatdad.pr-nanny`). After upgrading, reconnect GitHub in Settings, turn notifications on again (macOS asks for permission once more), and re-enable Launch at login. Settings, layout and bells carry over.
- Notifications are off until you turn on **Allow notifications**, and silent unless you turn on sound.
- CodeQL analyzes Go, Swift and the JavaScript/TypeScript in `site/`.
- CI validates through `alexcatdad/ci-actions` on macOS; `task check` runs every gate locally.

### Removed

- The Wails v3 app and Svelte frontend, its webview security layer and the Node/pnpm toolchain for the app.
- Linux and Windows builds, packaging, scripts and platform code. PR Nanny is macOS-only; earlier Linux and Windows assets stay available as release history, and the Linux Homebrew formula is no longer updated.
