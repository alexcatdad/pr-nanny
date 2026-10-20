# PR Nanny

Passive GitHub.com PR monitor. Never add a GitHub mutation, subscription change, telemetry, or frontend network/auth access.

- Go packages under internal/ implement domain logic. Wails adapters remain in the root.
- Pin Wails v3.0.0-beta.33. Production builds use `-tags production` and never include MCP or DevTools.
- Keep secrets in Go and the native credential store; no plaintext fallback or raw upstream error logging.
- Render API data as text, validate GitHub PR URLs before opening them.
- Test with synthetic fixtures, never real credentials.
- Run `go test -race ./internal/...`, frontend checks, and a production native build before delivery.
- Respect existing work and coordinate file ownership among agents. Do not wait for CI; use one actionable status check and report pending external prerequisites.
