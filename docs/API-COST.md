# GitHub API cost per polling cycle

All numbers come from `internal/github/cost_test.go` (`go test ./internal/github -run TestCost -v`), which drives the real client against a deterministic synthetic GraphQL server (`harness_test.go`). No real credentials or network are involved.

## Method and caveats

- **Requests, bytes**: counted by the harness (response bytes, uncompressed).
- **Points** are an estimate, not a measurement. The client does not request `rateLimit { cost }`, so the harness applies GitHub's documented node-limit formula: sum the requests needed for each connection at every level (a nested connection costs one request per parent node, using the `first` argument, not the result size), divide by 100, round, minimum 1. Search with `first:100` and three nested `first:100` connections is `1 + 3*100 = 301` requests, about 3 points even when the result is empty. `nodes(ids:[n])` is `1 + 3n`. An ID-only search page is 1 point. Real costs may differ slightly; verify with `rateLimit { cost }` before relying on absolute values. The primary GraphQL budget is 5,000 points/hour per user.
- **Latency** is simulated: requests run sequentially, so latency is requests x an assumed 150 ms round trip. `wall` is the real in-process time and only shows client overhead. Parallel searches were not tried because they raise secondary-rate-limit risk.
- World: 3 authored, 3 direct-review, 4 PRs per team (every fifth also requested from the next team, so results overlap), 5 tracked closed PRs.
- Polling stays at 60 s. Team list cache TTL stays at 5 minutes.

## Before (one full search per team)

| Teams | Cache | Requests | Points/cycle | Points/hour at 60 s | Bytes | Latency (sim.) |
|---|---|---|---|---|---|---|
| 0 | warm | 3 | 7 | 420 | 7.6 KB | 0.45 s |
| 5 | warm | 8 | 22 | 1,320 | 28 KB | 1.2 s |
| 25 | warm | 28 | 82 | 4,920 | 110 KB | 4.2 s |
| 50 | warm | 53 | 157 | 9,420 | 213 KB | 8.0 s |
| 50 | cold or expired | 55 | 159 | | 214 KB | 8.3 s |

At 25 teams a user already spends about 98% of the hourly budget on polling alone, and at 50 teams the budget is exhausted in about 32 minutes. This was judged excessive.

## After (team searches return IDs only; one batched `nodes()` lookup shared with tracked IDs)

| Teams | Cache | Requests | Points/cycle | Points/hour at 60 s | Bytes | Latency (sim.) |
|---|---|---|---|---|---|---|
| 0 | warm | 3 | 7 | 420 | 7.6 KB | 0.45 s |
| 5 | warm | 8 | 12 | 720 | 25 KB | 1.2 s |
| 25 | warm | 29 | 35 | 2,100 | 94 KB | 4.4 s |
| 50 | warm | 55 | 63 | 3,780 | 180 KB | 8.3 s |
| 50 | cold or expired | 57 | 65 | | 181 KB | 8.6 s |

Cold or expired cache costs two extra requests (organizations plus teams per organization; one organization here). Paginated case (2 teams x 250 PRs, 130 authored): 12 requests / 30 points before, 16 requests / 31 points after, so large result sets are neutral.

### Why this is safe

The same `is:pr is:open team-review-requested:<org/team>` searches run with the same 100-result paging and the same 1,000-result guard; only the selected fields change. Details come from the existing `nodes(ids:)` path, which already completes review-request, review and check pagination and normalizes identically. IDs already fetched by the authored/direct searches or already tracked are not fetched twice. `TestCostMatrix` and `TestCostPaginationAndOverlap` assert, against ground truth, complete discovery (open and closed), `Authored`, `ReviewRequested` and request node IDs for every scenario, including overlapping teams, 250-PR paged teams and tracked closed PRs.

Tradeoff: request count and latency do not drop (they rise by 1-2 requests), only points and bytes.

## Not done

- **OR-grouping team qualifiers in one search**: could cut requests to about 3 per cycle, but GitHub's semantics for repeated or OR-ed `team-review-requested` qualifiers (and the shared 1,000-result cap) cannot be verified offline with synthetic fixtures, so equivalence cannot be proven here. Revisit with a recorded real response.
- **Parallel team searches**: would cut latency, but increases secondary-rate-limit exposure.
- **Skipping the ID search for teams with no requests**: no GitHub signal exists without querying.

## Secondary rate limit behavior

A 403 with `Retry-After` and "secondary rate limit" text is classified as a rate limit (`RateLimitError`, default 60 s, honoring `Retry-After` up to 24 h). The cycle aborts at the first throttled request, returns no partial result, and the monitor keeps the last good state and does not advance notification baselines (`TestCostSecondaryRateLimit`: 50 teams, throttle after 60 points; aborted with exactly 1 throttled request). Team discovery treats rate limits as transient and never as a permission problem.

## Team discovery cost and permission degradation

Discovery costs `1 + organizations` requests (1 point each) per 5-minute TTL, about 1/5 of cycles. When discovery is denied, the degraded result is cached for one TTL as well (no per-minute retries of denied calls). The last complete team list is reused as a stand-in for up to 1 hour (`teamStaleGrace`) and is dropped immediately on HTTP 401.
