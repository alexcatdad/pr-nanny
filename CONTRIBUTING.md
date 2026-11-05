# Contributing

Keep PR Nanny a lightweight, single-account, read-only GitHub.com monitor. Start with [SPEC.md](SPEC.md), [ARCHITECTURE.md](ARCHITECTURE.md), and [SECURITY.md](SECURITY.md). Use synthetic fixtures and concise Conventional Commit messages (`fix:`, `test:`, `ci:`, `docs:`, `chore:`). Describe the observable problem, the change, and checks actually run in each pull request.

## Toolchain

Use Go 1.27.3 (`go.mod`), Node.js 24.19.0, pnpm 11.25.0 (corepack reads `packageManager`), [Task](https://taskfile.dev) 3, and pinned Wails `v3.0.0-beta.33`, on macOS 13+ with Apple Silicon and the Xcode command-line tools. Frontend dependencies are exact versions in `frontend/pnpm-lock.yaml`. PR Nanny is macOS-only.

## Local checks

```sh
task check   # lint, build, test, test:integration, security - everything CI runs
```

| Task | Runs |
|---|---|
| `task lint` | gofmt check, go vet, pinned golangci-lint, ShellCheck, actionlint, zizmor, Biome and svelte-check |
| `task build` | production `bin/pr-nanny-helper` and Wails `bin/pr-nanny`, then `scripts/check-production.sh` |
| `task test` | race-enabled Go tests with the 80% `internal/` coverage floor (`scripts/coverage-go.sh`), helper and native adapter tests, Vitest |
| `task test:integration` | Playwright browser tests against the mock bridge (installs Chromium) |
| `task security` | gitleaks, `go mod verify`, govulncheck, gosec and `pnpm audit` |

`sh scripts/install-ci-tools.sh` (called by `task lint`) downloads digest-verified ShellCheck 0.11.0 and zizmor 1.31.1 into `.tools/`. The Go tools come from the locked modules in `tools/lint` and `tools/security` or exact `go run` versions. Formatting is never rewritten by checks; use `gofmt -w` or `pnpm exec biome check --write .` from `frontend/`.

Playwright's test build includes synthetic fixtures and replaces `frontend/dist`, so build production assets again (`task build`) before packaging. Never package a fixture build. Never run credential-store roundtrip tests (`PR_NANNY_TEST_NATIVE_VAULT=1`) against your normal Keychain; ordinary Go tests use doubles.

## Backend helper

`cmd/pr-nanny-helper` is the backend for the coming native menu-bar app. It shares `internal/` with the Wails app, stores data in the same directory, and takes the same instance lock, so only one of them runs at a time. Its socket protocol is in [docs/HELPER-API.md](docs/HELPER-API.md). For local experiments use a throwaway directory and a PAT-mode `settings.json`, so it never reads your real `gh` session:

```sh
go run ./cmd/pr-nanny-helper -data-dir "$(mktemp -d /tmp/pn-XXXX)"
```

## Production packaging

`VERSION` is the version of ordinary development builds. Released builds take the version reserved by the local release tooling through `RELEASE_VERSION`; `scripts/read-version.sh` and the frontend build prefer it when set. Do not bump `VERSION` for a release. See [Releases](#releases) and [build/README.md](build/README.md).

```sh
task build
sh scripts/package.sh "$(sh scripts/read-version.sh)" darwin
sh scripts/verify-package.sh dist
```

Packaging never publishes a release. `--version` reports the application version and embedded VCS revision. Production builds exclude MCP, DevTools, development fixtures and server mode. Never combine the development-only `mcp` build tag with `production`.

## CI

`.github/workflows/validation.yml` calls `alexcatdad/ci-actions` (pinned to a commit SHA) on `macos-26`: pull requests run `pr / leaks`, `pr / lint`, `pr / build` and `pr / deps`; pushes to main and a weekly schedule run `main / test`, `main / security` and the integration task; CodeQL analyzes Go and JavaScript/TypeScript. Commands are the Task targets above, run through a pinned `go run` of go-task because the runner image has no Task binary. CI validates only: it never builds release artifacts, signs, publishes, tags or updates the tap. `pages.yml` builds and deploys the site.

The application has no telemetry, GitHub mutations, subscription changes, remote frontend code, or plaintext credential fallback. Keep OS permissions minimal and redact upstream errors.

## Releases

Releases are started only by an explicit local invocation on the coordinator Mac with [release-tools](https://github.com/alexcatdad/release-tools) `reltool`, pinned in `Taskfile.yml` (`RELTOOL_TAG` and `RELTOOL_COMMIT`). Nothing in GitHub Actions releases, signs, tags, dispatches or updates the tap. Check out `release-tools` next to this repository (`../release-tools`, tags fetched) and install [Go Task](https://taskfile.dev), Node 24.19 and pnpm 11.25.

Machine-specific settings (state directory, signing identity, notary profile name, optional SSH workers) live in `~/.config/release-tools/config.json`, outside any checkout. A release needs this file. Its `signing.pr-nanny` entry must name the Developer ID Application identity and the `notarytool` keychain profile; see `docs/consumer.md` and `examples/local/config.json` in release-tools.

```sh
task release:check    # read-only preflight: clean tree, next version, targets, signing identity, notary profile
task release:prepare  # reserve a version, run checks, build, sign, notarize, staple; publishes nothing
task release          # prepare, then draft release, SHA-256 verified upload, publish, tap update
task release:resume   # continue an interrupted release with the same version and artifacts
task release:abandon -- VERSION   # drop an unpublished reservation
```

Versions are CalVer `vYYYY.MM.DD.N` from the next release (release-tools allocates them; earlier `0.1.x` and `YYYY.M.N` tags remain as history). The only target is declared in `release.json`:

| Target | Artifact | Builds on |
|---|---|---|
| `darwin-arm64` (macOS 13+) | `pr-nanny-VERSION-darwin-arm64.zip`, signed, notarized, stapled | the coordinator Mac |

The macOS build task signs with the hardened runtime using `MACOS_SIGNING_IDENTITY` (and `MACOS_ENTITLEMENTS` when configured); `reltool` verifies the signature, notarizes and staples. The Homebrew cask is updated from the published release.
