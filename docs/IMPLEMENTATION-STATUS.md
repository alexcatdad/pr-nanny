# Engineering quality checkpoint

Updated: 2026-10-20. Base: `4697f8b025a3cc86615a126d7983997c460e7ce4` (RC1).

## Completed locally

- Single strict SemVer source (`VERSION`), runtime/UI metadata, generated Windows resources, versioned native packaging and collision-safe publication.
- Exact-main-source qualification now includes frontend, core coverage, lint, CodeQL, all three native platforms and a stable aggregate. Archive publication requires matching source/workflow attestations.
- Biome 2.5.15 covers Svelte 5, TypeScript, JSON and CSS; svelte-check remains active. Go formatting/vet/golangci-lint, ShellCheck, actionlint and zizmor are enforced.
- Go race coverage: 82.8% core statements, with an 80% floor. Frontend: 87.58% lines, 68.38% branches, with a 75% line floor. Retained reports include HTML, LCOV/JSON and CI summaries.
- All 11 frontend unit tests and 10 browser tests pass locally. Native macOS production race tests, packaging, archive verification and development/MCP/fixture exclusion pass.
- Gitleaks history scan, Govulncheck and Gosec core/native scans pass. Source/workflow security review found no exploitable metadata-to-script entrypoint or new security regression; the proven existing post-compromise navigation limit is documented in SECURITY.md.
- Deterministic 600-cycle polling soak passed repeated outages, notification deduplication and restart persistence; this simulates ten hours of polling time without claiming ten hours of OS execution.
- Existing-tap [PR #6](https://github.com/alexcatdad/homebrew-tap/pull/6) supplies qualified RC1 manifests, a verified release importer and reviewed update workflow. Ruby, Homebrew style, shell and workflow checks pass locally.
- Server-side v* tag update/deletion protection and immutable releases are enabled. Main retains its five original required checks plus strict `Qualification`; administrator enforcement remains enabled, with no mandatory human review.

## Baseline and measurement

- [RC1 qualification run](https://github.com/alexcatdad/pr-nanny/actions/runs/39384882982): 07:11:15–07:16:11 UTC, 296 seconds wall time; macOS was the longest job at 288 seconds. Native gosec compilation cost 53–93 seconds.
- CI now caches dependency-keyed pnpm stores and Go module/build dependencies and reuses one production frontend artifact. Optimized completed-run timing is still pending; no speedup is claimed yet.
- Local macOS package: 4,277,842 bytes before final source changes. Isolated bundled launch/frontend IPC probe: 1.07 seconds including its 500 ms verification delay; main-process maximum RSS 120,438,784 bytes. This excludes WebKit helper accounting and does not measure steady idle resources.

## Pending external qualification

- Implementation PR and exact-head independent review evidence to be recorded after integration.
- Windows/Linux native execution, Windows PE resource assertions, package install/uninstall probes, CodeQL uploads, and main-build attestations require completed GitHub Actions runs.
- RC2 remains unpublished until all release gates pass for its exact commit. RC1 is unchanged.
- Automatic upstream-to-tap dispatch requires `HOMEBREW_TAP_ACTIONS_TOKEN`, a fine-grained token scoped only to the existing tap with Actions: write. No credential was created. The tap update can be manually dispatched meanwhile.
- No Developer ID/Authenticode signing credentials have been assumed available. Physical desktop acceptance, later-login behavior and a fully interactive Windows session remain separate from automated qualification.

## Next executable actions

Publish the locally validated quality PR, record its checks and independent reviews, integrate the static site/community documentation separately, and validate completed native, release and Pages results. Use native auto-merge only after required checks and actionable review findings are resolved.

## Integration CI

[First quality run](https://github.com/alexcatdad/pr-nanny/actions/runs/39391536852) passed macOS and Linux native qualification, frontend, coverage, lint and security jobs. Windows resource generation exposed CRLF checkout handling; a reproducing local test now covers LF/CRLF input. CodeQL Go rejected the unsupported `none` build mode; its workflow now performs a traced production build with native dependencies and the shared frontend artifact. These fixes require a fresh exact-head run. The stale-poll browser regression now pauses the clock and was verified to reject removal of the save-generation guard.

## Delivery checkpoint — 08:40 UTC

- [Quality PR #5](https://github.com/alexcatdad/pr-nanny/pull/5) is ready for review with native auto-merge enabled; publication still depends on required checks. Head `5817b1d` replaces PowerShell host-only status output after the enforced analyzer reported warnings. Independent review found no blocker in that delta.
- [Second quality run](https://github.com/alexcatdad/pr-nanny/actions/runs/39392719790) passed both CodeQL languages, all security/lint/coverage/frontend jobs, and macOS/Linux native qualification. Windows reached PowerShell analysis and failed on two output warnings; the subsequent corrective head requires fresh qualification. No failing gate was disabled.
- Initial [Homebrew integration PR #6](https://github.com/alexcatdad/homebrew-tap/pull/6) merged as `8210d13`. Its manifests point to qualified RC1 artifacts. The tap's Actions setting permits the update workflow to create manifest PRs; workflow token defaults remain read-only. Additional real Homebrew install/uninstall CI is being prepared separately.
- [Site/community PR #6](https://github.com/alexcatdad/pr-nanny/pull/6) is stacked on the quality branch. Astro static output, dependency audit and local asset/link validation pass. Rendered checks at 390, 661, 680, 699, 900 and 1440 pixels show no horizontal overflow; keyboard focus contrast is 4.97:1 light and 8.47:1 dark. Pages deployment requires integration into main.
- Independent source security review of the integrated runtime/workflow/site changes found no actionable new vulnerability. Quality review verified the CRLF fixture, traced Go CodeQL build and deterministic stale-poll regression. Maintainer feedback on site copy, dark links, focus and tablet layout was addressed.

Next: finish the Homebrew installation checks, integrate the site after quality gates permit, verify the exact-main RC2 publication and deployed Pages result, then record completed-run timing and artifacts. Missing cross-repository dispatch credentials remain an explicit setup prerequisite rather than an invented secret.
