# Quality evidence

Local integration checks on 2026-10-20 used macOS ARM64. Hosted platform qualification is recorded in Actions and the implementation checkpoint; a local build does not establish Windows/Linux behavior.

## Coverage

Race-enabled Go core coverage is 82.8% of statements. Deduplicated package coverage from the combined profile:

| Package | Statements |
| --- | ---: |
| app | 85.8% |
| auth | 82.2% |
| core | 85.8% |
| github | 78.4% |
| model | 100.0% |
| notify | 88.0% |
| storage | 85.0% |
| websecurity | 96.8% |

Frontend V8 coverage: 87.58% lines, 88% statements, 68.38% branches and 85.47% functions. Settings authentication success paths are exercised through rendered component tests; Settings line coverage is 93.1%. Coverage includes application/components, bridge and formatters, with synthetic test fixtures outside the production app.

`sh scripts/coverage-go.sh` generates an atomic race profile, HTML and function summary with an 80% core floor. `pnpm test:coverage` in frontend generates HTML, LCOV and JSON summaries with a 75% line floor. CI retains both reports and publishes readable job summaries.

## Targeted reliability evidence

The 600-cycle deterministic polling soak advances ten hours of simulated polling without sleeping. It checks repeated offline recovery, preservation of the last successful snapshot, bounded persisted identities, notification attempts and restart deduplication. It does not establish ten hours of OS uptime.

The stale-poll browser regression pauses the browser clock, holds an old response, saves a new layout and then releases the response. A temporary mutation removing the save-generation guard made this test fail; restoring the guard made it pass. The temporary mutation was removed and production assets rebuilt.

Windows resource generation has an isolated Go regression for LF/CRLF checkout line endings. Native package verification checks exact runtime versions; Windows additionally checks the PE's numeric file version and full SemVer product version. Real Windows execution remains a hosted-runner check.

## Local native measurement

One isolated bundled macOS launch/frontend IPC probe completed in 1.07 seconds, including its deliberate 500 ms verification delay. Main-process maximum RSS was 120,438,784 bytes; WebKit helper memory is excluded. This is neither a cold-start benchmark nor a steady idle RAM/CPU measurement. The intermediate macOS archive was 4,277,842 bytes; final release sizes come from qualified artifacts.

## Security and qualification

Source history is scanned with Gitleaks; workflows with zizmor/actionlint; application code with Gosec, Govulncheck and CodeQL. Exact-head source review found no new exploitable entrypoint in the reviewed runtime boundaries. SECURITY.md records the demonstrated existing post-compromise webview-navigation limitation. Native signing/notarization and physical desktop acceptance remain distinct from build provenance and automated qualification.

Release tags reject updates/deletion, new releases are immutable, and publication verifies all required jobs plus all package attestations for the exact source SHA. See [the implementation checkpoint](IMPLEMENTATION-STATUS.md) and [release documentation](../build/README.md).
