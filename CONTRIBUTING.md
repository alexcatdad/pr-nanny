# Contributing

## Requirements

Use the pinned toolchain: Go 1.27.2 (`go.mod`), Node.js 24.19.0, pnpm 11.25.0, TypeScript/Svelte dependencies from `frontend/pnpm-lock.yaml`, and Wails `v3.0.0-beta.33`. Use the operating system's native GUI prerequisites to build a Wails app:

- macOS packaging is supported on an Apple Silicon host with macOS 13+ SDK/runtime frameworks.
- Windows packaging must run on Windows x64 and requires WebView2 on the target system.
- Linux packaging must run on Linux x86_64 with GTK4 and WebKitGTK 6.0 development files, plus Secret Service/runtime dependencies. The CI package baseline is Ubuntu 24.04 (glibc 2.39); Fedora must provide glibc 2.39 or newer and the same GTK/WebKit/Secret Service stack.

The Windows and macOS/Linux packaging scripts are host-native; they are not cross-compilers. Do not change lockfiles casually or use floating framework versions.

## Checks

From the repository root:

```sh
cd frontend
pnpm install --frozen-lockfile
pnpm check
pnpm test
pnpm audit --audit-level=low
pnpm build
pnpm exec playwright install chromium
pnpm test:e2e
cd ..
go test -race ./internal/...
go vet ./internal/...
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./internal/...
go -C tools/security build -o ../../gosec github.com/securego/gosec/v2/cmd/gosec
./gosec ./internal/...
```

The security helper is pinned in its own `tools/security` Go module so its `golang.org/x/tools` compatibility requirements do not change the app module's dependency graph.

The Playwright bridge uses deterministic synthetic fixtures. Never add real credentials or private PR data to fixtures, screenshots, logs, or tests. Go tests use test doubles; native credential-store roundtrip tests require an explicitly isolated test vault and are not part of the ordinary local test command.

To verify a production build on a native build host, build frontend assets first, then build the root package:

```sh
(cd frontend && pnpm install --frozen-lockfile && pnpm build)
go build -tags production -trimpath -o bin/pr-nanny .
go test -race -tags production .
sh scripts/check-production.sh bin/pr-nanny
```

On Windows, use `bin/pr-nanny.exe` and the PowerShell scripts. Production packaging is host-specific:

```sh
sh scripts/package.sh 0.1.0-rc.1 darwin  # macOS ARM64 host
sh scripts/package.sh 0.1.0-rc.1 linux   # Linux x86_64 host
./scripts/package-windows.ps1 -Version 0.1.0-rc.1  # Windows x64 host
```

The macOS/Linux packaging script builds the frontend before compiling. Run `sh scripts/verify-package.sh dist` after producing packages on macOS/Linux; run `./scripts/verify-package-windows.ps1 -Archive dist/pr-nanny-0.1.0-rc.1-windows-x64.zip` on Windows. Packaging writes archives and SHA-256 sidecars under `dist/`; it does not publish them.

## Development bridge and fixtures

Ordinary browser tests install the mock bridge with synthetic fixtures. A standalone Vite page without that bridge cannot perform authenticated API work. Native development uses the Wails runtime and `Call.ByName` against the explicit methods on `main.Service`; there is no generated binding step.

The development-only `mcp` tag enables the Wails MCP interface and its exact configured loopback callback origin. It must never be combined with production. Production build checks enforce the tag boundary and inspect the binary. The production packaging commands always pass `-tags production`. `scripts/homebrew.sh` can emit a macOS cask template and/or Linux formula template once real HTTPS artifact URLs and SHA-256 values exist; it does not publish releases or a tap.

## CI and platform claims

The validation workflow runs frontend checks, race-enabled Go tests, security checks, native production builds, Wails launch/IPC smoke tests, and package verification on macOS ARM64, Windows x64, and Ubuntu x64. A launch smoke test is not proof of physical tray interaction or of Windows desktop behavior on an untested host. Report only checks that actually completed successfully; do not treat a pending workflow or local cross-compile as a platform qualification.

Windows and Linux CI also invoke `--desktop-smoke-test` on the actual application after embedded frontend readiness. The probe uses a unique synthetic app identity, checks native tray hide/show, verifies autostart registration and removal, and sends a native notification without requesting permissions. Linux invokes the exported StatusNotifierItem activation method over D-Bus and verifies delivery through Dunst. The flag refuses execution outside GitHub Actions and on macOS; macOS notification permission and bundled autostart behavior need separate desktop qualification. These probes do not simulate a physical panel click or a subsequent login.

## Scope

Keep changes within PR Nanny's passive, read-only boundary. Do not add GitHub mutations, telemetry, subscription changes, extra accounts, a database, external services, or production fixture data. Keep secrets in Go and native credential storage. Keep API values rendered as text and validate URLs before opening them.
