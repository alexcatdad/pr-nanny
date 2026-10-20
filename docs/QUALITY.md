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

Frontend V8 coverage: 87.63% lines, 88% statements, 68.15% branches and 85.47% functions. Settings authentication success paths are exercised through rendered component tests; Settings line coverage is 93.1%. Coverage includes application/components, bridge and formatters, with synthetic test fixtures outside the production app.

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

### Request cost after the team-search change

Team review searches now return PR node IDs only and details come from one batched `nodes(ids:)` lookup shared with tracked IDs. Estimated GraphQL points per warm-cache cycle (node-limit formula, not measured `rateLimit` cost): 0 teams 7, 5 teams 22 to 12, 25 teams 82 to 35, 50 teams 157 to 63. At 60 s polling that is 9,420 to 3,780 points/hour at 50 teams against the 5,000/hour budget. Request counts and simulated latency are unchanged; complete discovery and result IDs are asserted against ground truth. Method, caveats and the options not taken (OR-grouped and parallel searches) are in [API-COST.md](API-COST.md).

## Restricted organization permissions

A denied team lookup (HTTP 403, `FORBIDDEN`, `INSUFFICIENT_SCOPES`, `SAML_ENFORCEMENT`) now degrades only team review discovery through a narrow `ErrPermission`; 401 and `UNAUTHORIZED` still fail as authentication, and the token's account is re-verified when the organization list is denied. The UI shows a fixed warning with no organization or team names. Previously seen team requests are carried forward while degraded, so loss and recovery raise no removed or new-request notifications. Synthetic scenarios cover full permissions, no membership, forbidden team listing, restricted token, revocation mid-session, restoration, stale cache (1 hour bounded grace), partial GraphQL errors and unaffected authored/direct-review PRs. An independent review found no blocking issue; the accepted residual risk is that a user who left a team may still see its requests for up to an hour while discovery stays denied.

## Frontend bridge traffic

The popup previously called `GetSnapshot()` every second (3,601 calls/hour measured with a counting mock bridge under fake timers). It now ticks the relative-time clock locally and syncs every 5 s only while visible, immediately on becoming visible or focused: 721 calls/hour visible, 1 hidden. Ten deterministic tests cover clock-without-IPC, hidden silence, prompt updates, reopen refresh, stale-response guards and cleanup. Whether each platform's webview fires `visibilitychange` on popup hide/show is not yet confirmed on real builds; the focus listener is the fallback.

## Homebrew installation

[Tap PR #7](https://github.com/alexcatdad/homebrew-tap/pull/7) passed all exact-head checks before merge, including real macOS ARM64 cask install/uninstall under a temporary Applications directory and Ubuntu 24.04 x64 formula install/test/uninstall. Homebrew verified the qualified RC1 archive checksums selected by the manifests. These checks establish package deployment/removal through the existing tap, separately from the native application launch and credential-store tests above.

## Final delivery and desktop qualification

The immutable [RC2 release](https://github.com/alexcatdad/pr-nanny/releases/tag/v0.1.0-rc.2) comes from `0b7e9fd1bbf75f552f3f16b93e770a39a8c40335`. [Exact-main validation](https://github.com/alexcatdad/pr-nanny/actions/runs/39394175301) and [publication](https://github.com/alexcatdad/pr-nanny/actions/runs/39394891992) passed, including source/workflow-bound package attestations. [Tap PR #8](https://github.com/alexcatdad/homebrew-tap/pull/8) imported RC2 through the existing non-bootstrap workflow and merged after all three manifest and real installation checks passed.

[CodeQL alert #1](https://github.com/alexcatdad/pr-nanny/security/code-scanning/1), `js/file-system-race` / CWE-367, reported `stat(candidate)` followed by `readFile(htmlPath)` in `site/scripts/check-links.mjs`. Independent security review established that this build-only checker reads HTML solely to compare anchor IDs. It neither executes, writes, transmits nor prints those file contents. Its CI caller already executes repository-controlled Astro code at the same runner privilege. Only `site/dist` is deployed, and the desktop embeds `frontend/dist`; the checker is absent from shipped runtime surfaces. The sequence can affect validation correctness under concurrent filesystem modification, but crosses no supported security boundary. The alert was dismissed through GitHub's supported false-positive review mechanism with this evidence; the CodeQL check became successful and no open code-scanning alerts remained at the integration check. No scanner or rule was disabled.

[Website PR #6](https://github.com/alexcatdad/pr-nanny/pull/6) merged as `7421cfd` after all mandatory checks passed. The [live Pages site](https://alexcatdad.github.io/pr-nanny/) returned 200 and rendered correctly at desktop 1440×1000 and mobile 390×844, in light and dark mode. Visible assets loaded under `/pr-nanny/`; there were no horizontal overflows, console errors, runtime scripts or third-party requests. Keyboard focus, accessible link names, heading order, landmarks and the mobile Privacy anchor were checked. Published screenshots contain synthetic data.

On macOS ARM64, the actual RC2 release ZIP's SHA-256 and source/workflow attestation were verified before installation into the user's Applications directory. The ad-hoc signature verified, runtime `--version` matched RC2 and its source commit, and the real production app launched, reused the existing GitHub CLI session, discovered PRs, opened Settings and completed explicit CLI reconnection. Launch-at-login was toggled off/on through the production UI; macOS's login-item registry changed from disabled to enabled, restoring the original setting. This establishes registration, not a later login execution. Real-account PR titles and credentials were not published in test fixtures or reports.

Dogfooding reproduced the refresh text `Updated now ago`. The immutable [RC3 release](https://github.com/alexcatdad/pr-nanny/releases/tag/v0.1.0-rc.3), from fix commit `511dc3aa125da4749f11e76dfb5de2cbcc0bc83a`, changes that text to `Updated just now`; a rendered, clock-controlled regression first failed on RC2's behavior, then passed for both fresh and one-minute-old updates. RC2 remains unchanged.

[RC3 exact-main qualification](https://github.com/alexcatdad/pr-nanny/actions/runs/39409845781) passed all eleven jobs before publication. [Tap PR #9](https://github.com/alexcatdad/homebrew-tap/pull/9) imported it through the same source/attestation/checksum verifier and merged only after manifest/style and real macOS/Linux installation checks passed. Both tap manifests select `0.1.0-rc.3`. The downloaded macOS RC3 package was independently verified for checksum, exact source/workflow attestation, signature and runtime version, then installed. The real UI showed the corrected label and RC3 version, reconnected and retained preferences. Performance measurements below remain RC2 measurements.

The [RC2 Windows native job](https://github.com/alexcatdad/pr-nanny/actions/runs/39394175301/job/118532874621) passed credential-store roundtrip, embedded WebView2/IPC launch, popup hide/show via native tray methods, autostart enable/status/disable/status, notification authorization/two synthetic sends, PE/runtime/package integrity, Start Menu installation/removal and PowerShell analysis. It does not establish physical tray interaction or visual toast delivery. No Fedora KDE graphical session is exposed by this task's available host/surface configuration; no VM or infrastructure was provisioned for testing.

### Bounded production runtime sample

A 301-second real-time RC2 run on macOS ARM64 collected 21 process samples at 15-second intervals with live 60-second GitHub polling. Normal desktop use and popup/focus changes were possible during this window; this is not a controlled hidden-window idle benchmark. Main-process cumulative CPU increased by 1.63 seconds (0.54% of one core). Two same-user, near-simultaneously started WebKit processes increased by 1.36 seconds (WebContent, 0.45%) and 0.01 seconds (Networking, below 0.01%); combined average was approximately 1.00% of one core.

RSS ranged from 103.2–108.3 MiB for the app, 56.8–78.0 MiB for WebContent and 7.8–8.9 MiB for Networking (summed 168.2–191.9 MiB). Shared pages can be counted more than once. Main RSS remained approximately flat; the renderer grew about 21 MiB during the short window, which does not by itself demonstrate either a leak or prolonged stability. Helper association is correlated by user/start time rather than independently established. No telemetry or persistent monitoring service was added; the temporary sampler and raw CSV were removed.

After the copy change, local validation passes 11 unit tests, all 11 Playwright tests, frontend type checks, Go core race tests, production native adapter race tests, and the macOS production package/integrity/development-capability checks. Frontend coverage is 87.63% lines, 88% statements, 68.15% branches and 85.47% functions. The synthetic README and site screenshots were regenerated for the new text/version.

A separate 120-second hidden-popup polling sample collected nine readings at 15-second intervals after Escape dismissed the window. Main CPU increased by 0.26 seconds (0.22% of one core), WebContent by 0.36 seconds (0.30%) and Networking by less than the cumulative timer's 0.01-second granularity; combined average was 0.52%. RSS ranges were 102.2–107.2 MiB main, 74.5–80.5 MiB WebContent and 7.8–8.5 MiB Networking (summed 186.0–195.6 MiB, subject to shared-page double counting). This is bounded idle evidence, not a long-duration leak test.

### Manual desktop acceptance and shutdown

Physical menu-bar clicking was confirmed to anchor the popup beneath its icon. The centered opens observed when automation directly raised the app were not status-item clicks; no tray positioning change was made. A bell was enabled only for this task's public PR #9. Its live status changed from Running to Passed, and visible native “CI status changed” notification delivery was confirmed. The test bell was restored to its original off state; other bells were left unchanged. Escape dismissed the popup. Normal application-menu Quit removed the main process and both sampled WebKit processes, verified through process inspection.

Whole-system sleep/resume and actual network disconnection were not forced on the active desktop. The deterministic outage/recovery and polling-wakeup tests remain application-level evidence, not substitutes for those physical-session checks. Full Fedora KDE graphical acceptance remains unavailable in this task's exposed host/surface configuration.

Three fresh invocations of the installed RC2 bundle's isolated `--smoke-test` completed in 1.11, 1.07 and 1.07 seconds (median 1.07 seconds), including its deliberate 500 ms post-IPC verification delay. Main-process maximum RSS was 122,437,632 / 122,355,712 / 122,093,568 bytes. These measure bundled frontend/Go IPC readiness with temporary state and warm system caches; they do not measure authenticated cold-start completion or include WebKit helpers. A subsequent normal app restart reconnected through the saved CLI method, rediscovered PRs and retained the restored bell preference.
