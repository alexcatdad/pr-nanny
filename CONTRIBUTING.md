# Contributing

Keep PR Nanny a lightweight, single-account, read-only GitHub.com monitor. Start with [SPEC.md](SPEC.md), [ARCHITECTURE.md](ARCHITECTURE.md), and [SECURITY.md](SECURITY.md). Use synthetic fixtures and concise Conventional Commit messages (`fix:`, `test:`, `ci:`, `docs:`, `chore:`). Describe the observable problem, the change, and checks actually run in each pull request.

## Toolchain

Use Go 1.27.3 (`go.mod`), Xcode with Swift 6 (`swift format` ships with it), and [Task](https://taskfile.dev) 3 on an Apple Silicon Mac. The Swift package in `macos/` depends only on first-party [`alexcatdad/menubar-kit`](https://github.com/alexcatdad/menubar-kit), pinned by commit in `macos/Package.swift` (SwiftPM cannot resolve its four-part CalVer tags). PR Nanny is macOS-only; `site/` alone uses Node and pnpm.

## Local checks

```sh
task check   # lint, build, test, test:integration, security - everything CI runs
```

| Task | Runs |
|---|---|
| `task lint` | gofmt check, go vet, pinned golangci-lint, `swift format lint --strict`, ShellCheck, actionlint, zizmor |
| `task build` | production `bin/pr-nanny-helper` and `scripts/check-production.sh`, then the Swift app and tests with warnings as errors |
| `task test` | race-enabled Go tests with the 80% `internal/` coverage floor (`scripts/coverage-go.sh`), helper tests, `swift test` |
| `task test:integration` | builds the dev bundle, validates it, and runs `PRNanny --smoke-test` (bundled helper answers over its socket) |
| `task security` | gitleaks, `go mod verify`, govulncheck and gosec |
| `task dev:app`, `task dev:run` | build (and launch) `macos/.build/dev/PR Nanny Dev.app`, bundle ID `com.alexcatdad.pr-nanny.dev` |

`sh scripts/install-ci-tools.sh` (called by `task lint`) downloads digest-verified ShellCheck 0.11.0 and zizmor 1.31.1 into `.tools/`. The Go tools come from the locked modules in `tools/lint` and `tools/security` or exact `go run` versions. Formatting is never rewritten by checks; use `gofmt -w` or `swift format format --in-place --recursive macos/Package.swift macos/Sources macos/Tests`.

Never run credential-store roundtrip tests (`PR_NANNY_TEST_NATIVE_VAULT=1`) against your normal Keychain; ordinary Go tests use doubles.

## App and helper

`macos/` is the Swift menu-bar app; `cmd/pr-nanny-helper` is the Go backend it starts from `Contents/Helpers/`. Keep logic testable in `PRNannyNative` and the app target thin. The socket protocol is in [docs/HELPER-API.md](docs/HELPER-API.md). The dev bundle keeps its data in `~/Library/Application Support/pr-nanny-dev`, so it never touches an installed app; it starts disconnected and never reads your `gh` session until you press **Connect GitHub CLI**. For local experiments use a throwaway directory and a PAT-mode `settings.json`, so it never reads your real `gh` session:

```sh
go run ./cmd/pr-nanny-helper -data-dir "$(mktemp -d /tmp/pn-XXXX)"
```

## Production packaging

`VERSION` is the version of ordinary development builds. Released builds take the version reserved by the local release tooling through `RELEASE_VERSION`; `scripts/read-version.sh` prefers it when set. Do not bump `VERSION` for a release. See [Releases](#releases) and [build/README.md](build/README.md).

```sh
task build
sh scripts/package.sh "$(sh scripts/read-version.sh)" darwin
sh scripts/verify-package.sh dist
```

`scripts/package.sh` calls `macos/Scripts/package-app.sh`, which builds the Swift app and the production helper, assembles `PR Nanny.app` (`Contents/MacOS/PRNanny`, `Contents/Helpers/pr-nanny-helper`), validates it with `macos/Scripts/validate-bundle.sh`, and signs the helper first and the app last. Packaging never publishes a release. `pr-nanny-helper -version` reports the version and VCS revision.

## CI

`.github/workflows/validation.yml` calls `alexcatdad/ci-actions` (pinned to a commit SHA) on `macos-26`: pull requests run `pr / leaks`, `pr / lint`, `pr / build` and `pr / deps`; pushes to main and a weekly schedule run `main / test`, `main / security` and the integration task; CodeQL analyzes Go, Swift (`swift build --package-path macos`) and the JavaScript/TypeScript in `site/`. Commands are the Task targets above, run through a pinned `go run` of go-task because the runner image has no Task binary. CI validates only: it never builds release artifacts, signs, publishes, tags or updates the tap. `pages.yml` builds and deploys the site.

The application has no telemetry, GitHub mutations, subscription changes, remote code, or plaintext credential fallback. Keep OS permissions minimal and redact upstream errors.

## Releases

Releases are started only by an explicit local invocation on the coordinator Mac with [release-tools](https://github.com/alexcatdad/release-tools) `reltool`, pinned in `Taskfile.yml` (`RELTOOL_TAG` and `RELTOOL_COMMIT`). Nothing in GitHub Actions releases, signs, tags, dispatches or updates the tap. Check out `release-tools` next to this repository (`../release-tools`, tags fetched) and install [Go Task](https://taskfile.dev) and Xcode.

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

The macOS build task signs the helper and then the app with the hardened runtime using `MACOS_SIGNING_IDENTITY` (and `MACOS_ENTITLEMENTS` for the app when configured); `reltool` verifies the signature, notarizes and staples. The Homebrew cask is updated from the published release.
