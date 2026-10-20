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

## Hosted native evidence

[Validation run 39392719790](https://github.com/alexcatdad/pr-nanny/actions/runs/39392719790) completed macOS and Linux native jobs successfully. Both performed real native credential-store roundtrips, production capability checks, native launch with embedded frontend IPC, and package integrity verification. Linux additionally passed isolated install/reinstall/uninstall with paths containing spaces and preserved unrelated data. Its desktop probe ran under Xvfb/D-Bus with a test keyring and notification daemon. These are automated runner checks; they do not establish physical desktop or login-session behavior.

The same completed run restored primary-key Go caches on macOS and Linux; their native jobs took 49 and 102 seconds respectively, excluding the shared frontend dependency and runner scheduling. That run failed at Windows PowerShell lint. The corrected [run 39393580969](https://github.com/alexcatdad/pr-nanny/actions/runs/39393580969) passed every job, including Windows PE/resource/runtime checks, isolated installation/removal and PowerShell analysis. It completed in 264 seconds versus RC1's 296 seconds; Windows was its longest native job at 225 seconds. Both CodeQL language jobs passed, with Go extraction/analysis taking 146 seconds in parallel. These are different hosted runs with warm caches and PR/main attestation differences, not a controlled speed benchmark.

## Polling request budget and recovery limits

A successful fetch searches authored PRs, direct user review requests, and each discovered team independently, paginating at 100 results per page. The one-team deduplication fixture asserts three search calls and one output entry for a PR returned in both review scopes. Including discovery and a tracked-node batch, that cold fixture exercises six API requests. Team discovery is cached per account/client for five minutes; warm-fetch savings are implementation behavior without a dedicated request-count assertion. Additional result pages and review/check continuations add requests; tracked identities are fetched in batches of 100 (the 101-ID regression asserts batches of 100 and one).

The polling loop regression verifies interval-change wakeup and cancellation. The deterministic soak covers application-level outage recovery and bounded persisted identities; it does not measure goroutine growth, OS sleep/resume, idle CPU or whole-process-tree memory. Those remain separate native acceptance measurements.

## Homebrew installation

[Tap PR #7](https://github.com/alexcatdad/homebrew-tap/pull/7) passed all exact-head checks before merge, including real macOS ARM64 cask install/uninstall under a temporary Applications directory and Ubuntu 24.04 x64 formula install/test/uninstall. Homebrew verified the qualified RC1 archive checksums selected by the manifests. These checks establish package deployment/removal through the existing tap, separately from the native application launch and credential-store tests above.

## Final delivery and desktop qualification

The immutable [RC2 release](https://github.com/alexcatdad/pr-nanny/releases/tag/v0.1.0-rc.2) comes from `0b7e9fd1bbf75f552f3f16b93e770a39a8c40335`. [Exact-main validation](https://github.com/alexcatdad/pr-nanny/actions/runs/39394175301) and [publication](https://github.com/alexcatdad/pr-nanny/actions/runs/39394891992) passed, including source/workflow-bound package attestations. [Tap PR #8](https://github.com/alexcatdad/homebrew-tap/pull/8) imported RC2 through the existing non-bootstrap workflow and merged after all three manifest and real installation checks passed.

[CodeQL alert #1](https://github.com/alexcatdad/pr-nanny/security/code-scanning/1), `js/file-system-race` / CWE-367, reported `stat(candidate)` followed by `readFile(htmlPath)` in `site/scripts/check-links.mjs`. Independent security review established that this build-only checker reads HTML solely to compare anchor IDs. It neither executes, writes, transmits nor prints those file contents. Its CI caller already executes repository-controlled Astro code at the same runner privilege. Only `site/dist` is deployed, and the desktop embeds `frontend/dist`; the checker is absent from shipped runtime surfaces. The sequence can affect validation correctness under concurrent filesystem modification, but crosses no supported security boundary. The alert was dismissed through GitHub's supported false-positive review mechanism with this evidence; the CodeQL check became successful and no open code-scanning alerts remained at the integration check. No scanner or rule was disabled.

[Website PR #6](https://github.com/alexcatdad/pr-nanny/pull/6) merged as `7421cfd` after all mandatory checks passed. The [live Pages site](https://alexcatdad.github.io/pr-nanny/) returned 200 and rendered correctly at desktop 1440×1000 and mobile 390×844, in light and dark mode. Visible assets loaded under `/pr-nanny/`; there were no horizontal overflows, console errors, runtime scripts or third-party requests. Keyboard focus, accessible link names, heading order, landmarks and the mobile Privacy anchor were checked. Published screenshots contain synthetic data.

On macOS ARM64, the actual RC2 release ZIP's SHA-256 and source/workflow attestation were verified before installation into the user's Applications directory. The ad-hoc signature verified, runtime `--version` matched RC2 and its source commit, and the real production app launched, reused the existing GitHub CLI session, discovered PRs, opened Settings and completed explicit CLI reconnection. Launch-at-login was toggled off/on through the production UI; macOS's login-item registry changed from disabled to enabled, restoring the original setting. This establishes registration, not a later login execution. Real-account PR titles and credentials were not published in test fixtures or reports.

Dogfooding reproduced the refresh text `Updated now ago`. The next prerelease changes that text to `Updated just now`; a rendered, clock-controlled regression first failed on RC2's behavior, then passed for both fresh and one-minute-old updates. RC2 remains unchanged.

The [RC2 Windows native job](https://github.com/alexcatdad/pr-nanny/actions/runs/39394175301/job/118532874621) passed credential-store roundtrip, embedded WebView2/IPC launch, popup hide/show via native tray methods, autostart enable/status/disable/status, notification authorization/two synthetic sends, PE/runtime/package integrity, Start Menu installation/removal and PowerShell analysis. It does not establish physical tray interaction or visual toast delivery. No Fedora KDE graphical session is exposed by this task's available host/surface configuration; no VM or infrastructure was provisioned for testing.
