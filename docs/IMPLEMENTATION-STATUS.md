# Final qualification checkpoint

Updated: 2026-10-20. Working branch: `codex/final-qualification`.

## Delivered and verified

- [Quality PR #5](https://github.com/alexcatdad/pr-nanny/pull/5) merged as `0b7e9fd1bbf75f552f3f16b93e770a39a8c40335`. [Main validation](https://github.com/alexcatdad/pr-nanny/actions/runs/39394175301) passed all eleven jobs on that commit, including macOS ARM64, Windows x64 and Linux x64 native qualification. [RC2 publication](https://github.com/alexcatdad/pr-nanny/actions/runs/39394891992) succeeded with checksums and source/workflow-bound attestations. RC1 and RC2 remain immutable.
- Core race coverage: 82.8% statements. Frontend: 87.58% lines. Coverage floors, Biome/Svelte checks, Go lint, script/workflow analysis, Gitleaks, CodeQL, Govulncheck and Gosec remain enforced.
- [Website PR #6](https://github.com/alexcatdad/pr-nanny/pull/6) merged as `7421cfd`. [Live Pages](https://alexcatdad.github.io/pr-nanny/) returns the intended site. Actual desktop/mobile, light/dark, base-path assets, internal anchor, keyboard focus, semantic structure, no-script and no-third-party-request checks passed.
- [CodeQL alert #1](https://github.com/alexcatdad/pr-nanny/security/code-scanning/1), `js/file-system-race`, was independently reviewed as a demonstrable build-only false positive and dismissed through GitHub's supported mechanism. Its stat/read flow only compares generated HTML IDs; callers already execute repository-controlled build code at the same privilege. It is absent from shipped site/app surfaces. The CodeQL check became successful, and the integration check found no open code-scanning alerts. Full evidence is in [QUALITY.md](QUALITY.md).
- [Homebrew PR #8](https://github.com/alexcatdad/homebrew-tap/pull/8) merged as `c91fa9e02cb163caa4cb527585d960816cf2dd1d`. Both manifests on tap main now select qualified RC2. The existing non-bootstrap importer verified exact source, all three archive attestations and checksums; manifest syntax/style and real macOS/Linux install/uninstall checks passed before merge.
- Qualified RC2 was checksum/attestation/signature/version verified and installed in the user Applications folder on macOS ARM64. Real production launch, existing GitHub CLI authentication and reconnection, PR discovery, Settings navigation and browser URL opening were observed. Launch-at-login off/on changed the actual macOS login-item registry and restored the original enabled preference.
- The completed Windows native job verified WebView2/IPC, tray-method popup hide/show, autostart registration roundtrip, synthetic native notification calls, credential storage, PE/runtime integrity and Start Menu installation/removal. This does not establish physical tray clicks or visible toast delivery.

## Active final qualification

- Real dogfooding reproduced `Updated now ago`. Commit `05883c4` corrects it to `Updated just now`, with a rendered regression that failed before the fix and passes for fresh and one-minute-old updates.
- `VERSION` targets RC3 because the app changed. RC3 is not yet published; RC2 is not modified. Local integration checks, reviewed PR delivery and exact-main release qualification are still required.
- A bounded real-time production CPU/RSS sample is running. Results will distinguish main-process memory from correlated WebKit helpers and record the actual duration; the prior 600-cycle test remains ten simulated hours rather than ten hours of OS execution.
- Computer Use directly activates the app window. Those centered opens do not verify physical menu-bar anchoring; actual tray-click placement remains separate acceptance evidence.

## Remaining acceptance and prerequisites

- Finish real-time measurements and feasible production bell/notification, popup dismissal and clean-exit checks; record unavailable physical-session tests accurately.
- No Fedora KDE graphical session is exposed by the available host/surface configuration. No VM or infrastructure is provisioned for this task. Actual OS sleep/resume and later-login execution are not yet exercised.
- Deliver RC3 only through complete exact-main validation and package attestation. Import the qualified current prerelease through the existing tap workflow, then merge only after manifest and installation checks pass.
- Automatic cross-repository dispatch still requires `HOMEBREW_TAP_ACTIONS_TOKEN`, scoped to the tap with Actions: write. Manual dispatch works; no credential was created. Developer ID/Authenticode credentials remain unavailable, so provenance is distinct from signing/notarization.

See [QUALITY.md](QUALITY.md) for measured coverage, request budgets, security boundaries and native qualification limits.
