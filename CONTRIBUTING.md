# Contributing

Keep PR Nanny a lightweight, single-account, read-only GitHub.com monitor. Start with [SPEC.md](SPEC.md), [ARCHITECTURE.md](ARCHITECTURE.md), and [SECURITY.md](SECURITY.md). Use synthetic fixtures and concise Conventional Commit messages (`fix:`, `test:`, `ci:`, `docs:`, `chore:`). Describe the observable problem, the change, and checks actually run in each pull request.

## Toolchain

Use Go 1.27.2 (`go.mod`), Node.js 24.19.0, pnpm 11.25.0, and pinned Wails `v3.0.0-beta.33`. Frontend dependencies are exact versions in `frontend/pnpm-lock.yaml`. Install with `pnpm install --frozen-lockfile` from `frontend/`.

Native builds require their actual target OS:

- macOS ARM64: macOS 13+ SDK/runtime frameworks and Xcode command-line tools.
- Windows x64: native C compiler and Microsoft Edge WebView2 Runtime.
- Linux x64: GTK4, WebKitGTK 6 development packages, libsecret, an XDG desktop and Secret Service-compatible vault. Packages use Ubuntu 24.04/glibc 2.39 as their minimum binary baseline.

On Fedora, install the native build dependencies with:

```sh
sudo dnf install gcc pkgconf-pkg-config gtk4-devel webkitgtk6.0-devel libsecret-devel
```

The running app also needs a tray-capable desktop and an unlocked Secret Service
provider (for example KWallet with Secret Service enabled, or GNOME Keyring).
The isolated desktop tests additionally use `xorg-x11-server-Xvfb`, `dbus-daemon`,
`gnome-keyring`, and `dunst`. Do not run the CI-only desktop or vault probes on
your normal session or credential store. Fedora builds are for local testing;
build release archives on Ubuntu 24.04 to preserve the minimum binary baseline.

With Wails beta.33, tray popup positioning requires X11. On Wayland desktops
that provide XWayland, the app prefers the GTK X11 backend with a Wayland
fallback. An explicit `GDK_BACKEND` setting is respected; forcing `wayland`
or using a Wayland-only desktop leaves popup placement to the compositor.

## Local checks

Run the complete local lint/format/type-check entrypoint from the root:

```sh
sh scripts/lint.sh
```

It checks formatting without rewriting files, runs Go vet and pinned golangci-lint, ShellCheck, pinned actionlint, zizmor, Biome (including Svelte/CSS), and svelte-check. Install ShellCheck 0.11.0 and zizmor 1.31.1 on the local machine; the Go tools are built from locked tool modules. CI's Linux bootstrap downloads digest-verified tool archives with `sh scripts/install-ci-tools.sh`. Add `.tools` to PATH when using that bootstrap. Biome and Vitest use the frontend's pinned packages. Deliberate formatting fixes can use `gofmt -w` or `pnpm exec biome check --write .` from frontend.

```sh
sh scripts/coverage-go.sh
(cd frontend && pnpm test:coverage && pnpm test:e2e && pnpm build)
go test -race -tags production .
sh scripts/test-qualification.sh
gitleaks git --redact --no-banner --log-opts='--all'
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./internal/...
go -C tools/security build -o ../../gosec github.com/securego/gosec/v2/cmd/gosec
./gosec ./internal/...
```

Playwright needs Chromium (`pnpm exec playwright install chromium`). Its test build includes synthetic fixtures and replaces `frontend/dist`, so always build production assets again before native compilation. Never package a fixture build. A browser without a test bridge cannot perform authenticated API work.

Go coverage is race-enabled and atomic, with an 80% core floor, an HTML report and per-function text in `coverage/`. Vitest's V8 provider reports application logic coverage in `frontend/coverage/` with a 75% line floor. Generated/native framework glue is outside frontend coverage; tests cover the bridge, formatters and Svelte application/components. Both reports are retained as CI artifacts. These are correctness guardrails, not counts to inflate.

## Production packaging

`VERSION` is the sole release version source. Change it only for an intentional release. Ordinary documentation and dependency PRs need no version bump. See [build/README.md](build/README.md) for qualified publication and Homebrew.

```sh
(cd frontend && pnpm build)
go build -tags production -trimpath -o bin/pr-nanny .
sh scripts/check-production.sh bin/pr-nanny
sh scripts/package.sh "$(sh scripts/read-version.sh)" darwin
sh scripts/verify-package.sh dist
```

Use `linux` on a Linux x64 build host. On Windows use `scripts/package-windows.ps1` and `scripts/verify-package-windows.ps1`. Packaging never publishes a release. `--version` reports the application version and embedded VCS revision. Production builds exclude MCP, DevTools, development fixtures and server mode. Never combine the development-only `mcp` build tag with `production`.

## CI and desktop evidence

Validation runs frontend, lint, coverage, security, CodeQL and the three native targets. The stable `Qualification` job requires every validation job to succeed. Platform builds reuse the same run's production frontend artifact; test fixtures are built separately. No path filters skip release/security qualification.

Native jobs exercise the real packaged app, embedded frontend IPC, isolated native credential-store roundtrips, and package integrity. Windows and Linux also probe native tray hide/show, autostart registration/removal and notification dispatch. Linux verifies D-Bus activation and Dunst receipt. Windows exercises Start Menu installation/removal; Linux uses isolated XDG installation/removal. macOS uses a disposable test keychain.

An unattended runner does not establish physical tray clicks, a later login, notification delivery on every desktop, or an unlocked interactive Windows session. macOS notification permissions and bundled autostart need separate desktop acceptance. Do not claim unavailable physical testing. Never run credential-store roundtrip tests against a normal user vault; ordinary Go tests use doubles.

The application has no telemetry, GitHub mutations, subscription changes, remote frontend code, or plaintext credential fallback. Keep OS permissions minimal and redact upstream errors.
