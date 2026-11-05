# pr-nanny-helper API

`pr-nanny-helper` (`cmd/pr-nanny-helper`) runs PR Nanny's backend: GitHub polling, credentials, settings, state and notification gating. The native menu-bar app starts it as a child process (`Contents/Helpers/pr-nanny-helper`) and talks to it over a unix socket. The app owns everything the user sees: windows, posting notifications, opening URLs and launch at login.

## Process

- Data directory: `~/Library/Application Support/pr-nanny` (override with `-data-dir`). Mode 0700.
- Lock: `pr-nanny.lock` in that directory, an exclusive non-blocking `flock` held for the process lifetime. A second instance exits with status **3**.
- Socket: `pr-nanny.sock` in that directory, mode 0600. A stale socket is replaced only while the lock is held and only if it is a socket owned by the current user.
- Peers: every connection is checked with `LOCAL_PEERCRED`; a peer with a different UID gets `UNAUTHORIZED_PEER` and nothing is read.
- Lifetime: the helper exits when its parent process exits (kqueue `NOTE_EXIT`), on `SIGINT`/`SIGTERM`, or after replying to `stop`. If started with parent PID 1 it does not watch a parent.
- Output: nothing on success. Startup failures print one fixed line to stderr and exit 1. Upstream error text and secrets are never printed.
- `pr-nanny-helper -version` prints `pr-nanny-helper VERSION (COMMIT) protocol N`.

## Framing

One request per connection. Each frame is a 4-byte big-endian length followed by that many bytes of UTF-8 JSON, at most 1 MiB. The client writes one request frame and reads one reply frame. Requests have 5 seconds, `connectPAT` and `connectCLI` 35 seconds.

```json
{"version": 1, "id": "c0ffee", "op": "snapshot", "body": {}}
```

- `version`: protocol version, currently **1**. Anything else returns `PROTOCOL_VERSION`.
- `id`: 1–128 characters, echoed in the reply.
- `op`: operation name below.
- `body`: operation arguments. It may be omitted or `null` for operations without arguments. Unknown fields are rejected.

Replies carry exactly one of `body` or `error`:

```json
{"version": 1, "id": "c0ffee", "body": { }}
{"version": 1, "id": "c0ffee", "error": {"code": "SETTINGS", "message": "Unable to save settings. Check local storage and try again."}}
```

Error `message` values are fixed, user-safe sentences that the app may show as plain text.

| Code | Meaning |
|---|---|
| `INVALID_REQUEST` | Malformed frame, envelope or body |
| `PROTOCOL_VERSION` | Unsupported `version` |
| `UNKNOWN_OPERATION` | Unknown `op` |
| `UNAUTHORIZED_PEER` | Peer UID differs from the helper's |
| `SETTINGS` | Settings were rejected or could not be saved |
| `AUTHENTICATION` | GitHub authentication failed |
| `CREDENTIAL_STORE` | The Keychain is locked or unavailable |
| `UNAVAILABLE` | That pull request is unknown or its URL is not a canonical GitHub.com PR URL |
| `INTERNAL` | Anything else |

## Operations

`Snapshot` is `model.Snapshot` (`internal/model/model.go`): `settings`, `pullRequests`, `connection`, `message`, `lastRefresh`, `refreshing`, `degraded`. It never contains tokens or internal review-request IDs. All API text in it must be rendered as plain text.

| Op | Body | Reply |
|---|---|---|
| `snapshot` | – | `Snapshot` |
| `saveSettings` | `{"settings": Settings}` | `Snapshot`. Authentication method and account cannot change here. |
| `connectPAT` | `{"token": "…"}` | `Snapshot`. Validates the token, then stores it in the Keychain. |
| `connectCLI` | – | `Snapshot`. Uses the `gh` session for github.com. |
| `setBell` | `{"id": "PR node ID", "enabled": true}` | `Snapshot` |
| `setLayout` | `{"layout": "all" \| "tabs"}` | `Snapshot` |
| `refresh` | – | `{}`. Starts a fetch; read progress through `snapshot`. |
| `resolvePR` | `{"id": "PR node ID"}` | `{"url": "https://github.com/OWNER/REPO/pull/N"}`. Validated with `ValidPRURL`; the app validates again before opening. |
| `nextNotifications` | `{"cursor": "", "limit": 64}` | `{"notifications": [Notification], "cursor": "…", "missed": false}` |
| `health` | – | `{"status": "ok"}` |
| `version` | – | `{"version": "…", "commit": "…", "protocol": 1}` |
| `stop` | – | `{}`, then the helper shuts down. |

Only `refresh`, `connectPAT`, `connectCLI` and the monitor's own interval reach the GitHub API; the app should read cached snapshots and not poll `refresh`.

### Launch at login

The helper does not manage launch at login. `settings.launchAtLogin` is stored as a preference; the app applies it with `SMAppService.mainApp`.

### Notifications

The helper queues every monitor event and gates it **when `nextNotifications` reads it**: global switch, then that event's preference, then the PR's bell. Review requests bypass the bell. PRs no longer listed are dropped. An entry the gate rejects is consumed and not replayed.

```json
{"id": "32 hex chars", "prId": "PR node ID", "event": "ci", "title": "CI status changed", "subtitle": "OWNER/REPO #N", "body": "PR title"}
```

- `event` is one of `ci`, `approval`, `review_requested`, `merged`, `closed`; titles are fixed.
- `id` is the sha256-derived dedupe ID (same as the former Wails app). Use it as the notification identifier so repeats coalesce.
- `cursor` is opaque. Start with `""`; pass the returned cursor next time. A cursor from another helper run or a malformed one reads from the oldest retained entry.
- The queue keeps the newest 64 entries. `missed: true` means entries were dropped since the cursor.
- The app decides whether to post (permission, sound) and opens the PR with `resolvePR` on click.
