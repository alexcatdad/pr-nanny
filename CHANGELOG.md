# Changelog

All notable changes are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions are CalVer
`vYYYY.MM.DD.N`. Earlier releases are described in their
[GitHub release notes](https://github.com/alexcatdad/pr-nanny/releases).

## [Unreleased]

### Added

- `pr-nanny-helper`: the backend as a separate process with a versioned JSON API over a private unix socket, for the coming native menu-bar app ([docs/HELPER-API.md](docs/HELPER-API.md)).
- A single-instance lock on the data directory, shared by the app and the helper.

### Changed

- CI validates through `alexcatdad/ci-actions` on macOS; `task check` runs every gate locally.

### Removed

- Linux and Windows builds, packaging, scripts and platform code. PR Nanny is macOS-only; earlier Linux and Windows assets stay available as release history, and the Linux Homebrew formula is no longer updated.
