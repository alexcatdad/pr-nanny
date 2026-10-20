# Internal contract

Shared types: internal/model/model.go. No tokens in these types.

Frontend service is `main.Service`: GetSnapshot() (Snapshot,error), SaveSettings(Settings) (Snapshot,error), ConnectPAT(token string) (Snapshot,error), ConnectCLI() (Snapshot,error), SetBell(id string, enabled bool) (Snapshot,error), SetLayout(layout string) (Snapshot,error), Refresh() error, OpenPR(id string) error, HidePopup(). Settings view is frontend local navigation; tray Settings opens /?settings=1 or emits settings event. All bridge errors must be safe fixed messages. PAT input is ephemeral, never retained/persisted or returned; clear immediately after submission. Poll GetSnapshot every 1s for responsive state; backend owns GitHub polling.

Core packages use interfaces for test doubles. GraphQL client Fetch(ctx, token, account, trackedIDs) returns deduplicated current relevant open PRs and explicit closure/removed-review state for tracked IDs, not a partial snapshot silently. GitHub account validation is separate. Missing/inaccessible CI = not_available. Closure time is authoritative.

Writers: lead owns internal/model, root adapters, CI and documentation; core agent owns internal/core and internal/storage; GitHub agent owns internal/github and internal/auth; UI agent owns frontend only. Agents should report exported APIs before integrating. No agent may git add all or push; coordinator owns commits/integration.
