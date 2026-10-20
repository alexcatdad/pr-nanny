package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

const testToken = "ghp_012345678901234567890123456789"

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchDeduplicatesDirectTeamAuthoredAndTrackedPRs(t *testing.T) {
	var searches []string
	client := NewClient(&http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != endpoint {
			t.Fatalf("unexpected API request: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer "+testToken {
			t.Fatal("missing authorization header")
		}
		var payload graphRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var fixture string
		switch {
		case strings.Contains(payload.Query, "organizations(first:100"):
			fixture = `{"data":{"viewer":{"login":"octocat","organizations":{"nodes":[{"login":"acme"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`
		case strings.Contains(payload.Query, "organization(login:$org)"):
			fixture = `{"data":{"organization":{"teams":{"nodes":[{"slug":"engineering"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`
		case strings.Contains(payload.Query, "search(query:"):
			search, _ := payload.Variables["search"].(string)
			searches = append(searches, search)
			switch {
			case strings.Contains(search, "author:"):
				fixture = `{"data":{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[` + prFixture("PR_A", 1, "Authored", "octocat", "OPEN", "SUCCESS", false, false) + `]}}}`
			case strings.Contains(search, "user-review-requested:"):
				fixture = `{"data":{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[` + prFixture("PR_B", 2, "Direct review", "other", "OPEN", "IN_PROGRESS", true, false) + `]}}}`
			default:
				fixture = `{"data":{"search":{"issueCount":1,"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[` + prFixture("PR_B", 2, "Direct review", "other", "OPEN", "IN_PROGRESS", true, false) + `]}}}`
			}
		case strings.Contains(payload.Query, "nodes(ids:$ids)"):
			fixture = `{"data":{"nodes":[` + prFixture("PR_A", 1, "Authored", "octocat", "OPEN", "SUCCESS", false, false) + `,` + prFixture("PR_C", 3, "Closed", "other", "CLOSED", "FAILURE", false, true) + `,` + prFixture("PR_D", 4, "Tracked only", "other", "OPEN", "UNKNOWN", false, false) + `,null,` + prFixture("PR_F", 6, "Missed request search", "other", "OPEN", "SUCCESS", true, false) + `]}}`
		default:
			t.Fatalf("unexpected query: %s", payload.Query)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fixture))}, nil
	})})
	result, err := client.Fetch(context.Background(), testToken, "octocat", []string{"PR_A", "PR_C", "PR_D", "PR_E", "PR_F"})
	if err != nil {
		t.Fatal(err)
	}
	if len(searches) != 3 || !strings.Contains(strings.Join(searches, " "), "team-review-requested:acme/engineering") {
		t.Fatalf("did not search all review scopes: %v", searches)
	}
	if len(result.PullRequests) != 5 {
		t.Fatalf("expected 5 deduplicated PRs, got %d: %+v", len(result.PullRequests), result.PullRequests)
	}
	byID := map[string]model.PullRequest{}
	for _, pr := range result.PullRequests {
		byID[pr.ID] = pr
	}
	if !byID["PR_A"].Authored || byID["PR_A"].ReviewRequested {
		t.Fatalf("authored PR normalized incorrectly: %+v", byID["PR_A"])
	}
	if byID["PR_B"].Authored || !byID["PR_B"].ReviewRequested || byID["PR_B"].CI != model.CIRunning {
		t.Fatalf("direct/team PR normalized incorrectly: %+v", byID["PR_B"])
	}
	if byID["PR_C"].ClosedAt == nil || !byID["PR_C"].Merged || byID["PR_C"].CI != model.CINotPassed {
		t.Fatalf("closed PR lost state: %+v", byID["PR_C"])
	}
	if byID["PR_D"].ReviewRequested || byID["PR_D"].CI != model.CINotAvailable {
		t.Fatalf("tracked categoryless PR incorrect: %+v", byID["PR_D"])
	}
	if !byID["PR_F"].ReviewRequested {
		t.Fatalf("tracked direct review state was overridden by search index: %+v", byID["PR_F"])
	}
	if len(result.InaccessibleIDs) != 1 || result.InaccessibleIDs[0] != "PR_E" {
		t.Fatalf("inaccessible tracked IDs lost: %v", result.InaccessibleIDs)
	}
}

func TestFetchDoesNotReturnPartialSnapshotOnFailedPage(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var payload graphRequest
		_ = json.NewDecoder(req.Body).Decode(&payload)
		fixture := `{"data":{"viewer":{"login":"octocat","organizations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`
		if strings.Contains(payload.Query, "search(query:") {
			fixture = `{"data":{"search":{"issueCount":2,"pageInfo":{"hasNextPage":true,"endCursor":null},"nodes":[]}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fixture))}, nil
	})})
	result, err := client.Fetch(context.Background(), testToken, "octocat", nil)
	if !errors.Is(err, ErrResponse) || len(result.PullRequests) != 0 {
		t.Fatalf("partial snapshot returned: %+v, %v", result, err)
	}
}

func TestRateLimitErrorUsesSafeRetryHint(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		h := make(http.Header)
		h.Set("Retry-After", "75")
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: h, Body: io.NopCloser(strings.NewReader("secret upstream body"))}, nil
	})})
	err := client.request(context.Background(), testToken, "query{viewer{login}}", nil, &map[string]any{})
	var rate *RateLimitError
	if !errors.As(err, &rate) || rate.RetryAfter != 75*time.Second || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe or incorrect rate limit: %#v", err)
	}
}

func TestGraphQLRejectsIncompleteSuccessfulShapes(t *testing.T) {
	cases := []struct{ name, query, data string }{
		{"empty", `query{viewer{login}}`, `{}`},
		{"null", `query{viewer{login}}`, `null`},
		{"missing viewer", `query{viewer{login organizations(first:100){nodes{login} pageInfo{hasNextPage endCursor}}}}`, `{}`},
		{"missing org pageInfo", `query{viewer{organizations(first:100){nodes{login} pageInfo{hasNextPage endCursor}}}}`, `{"viewer":{"organizations":{"nodes":[]}}}`},
		{"missing team nodes", `query{organization(login:$org){teams(first:100){nodes{slug} pageInfo{hasNextPage endCursor}}}}`, `{"organization":{"teams":{"pageInfo":{"hasNextPage":false}}}}`},
		{"missing search pageInfo", `query($search:String!){search(query:$search,type:ISSUE,first:100){issueCount pageInfo{hasNextPage endCursor} nodes{__typename}}}`, `{"search":{"issueCount":0,"nodes":[]}}`},
		{"null search", `query($search:String!){search(query:$search,type:ISSUE,first:100){issueCount pageInfo{hasNextPage endCursor} nodes{__typename}}}`, `{"search":null}`},
		{"missing tracked nodes", `query($ids:[ID!]!){nodes(ids:$ids){__typename}}`, `{}`},
		{"missing review nodes", `query($id:ID!,$after:String){node(id:$id){... on PullRequest{reviewRequests(first:100,after:$after){nodes{requestedReviewer{__typename}} pageInfo{hasNextPage endCursor}}}}}`, `{"node":{"reviewRequests":{"pageInfo":{"hasNextPage":false}}}}`},
		{"missing check hasNextPage", `query($id:ID!,$after:String){node(id:$id){... on PullRequest{statusCheckRollup{contexts(first:100,after:$after){nodes{__typename} pageInfo{hasNextPage endCursor}}}}}}`, `{"node":{"statusCheckRollup":{"contexts":{"nodes":[],"pageInfo":{}}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":` + tc.data + `}`))}, nil
			})})
			var out map[string]any
			if err := client.request(context.Background(), testToken, tc.query, nil, &out); !errors.Is(err, ErrResponse) {
				t.Fatalf("malformed success accepted: %v", err)
			}
		})
	}
}

func TestTrackedIDsAreFetchedInBoundedBatches(t *testing.T) {
	var sizes []int
	client := NewClient(&http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var payload graphRequest
		_ = json.NewDecoder(req.Body).Decode(&payload)
		var fixture string
		switch {
		case strings.Contains(payload.Query, "organizations(first:100"):
			fixture = `{"data":{"viewer":{"login":"octocat","organizations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`
		case strings.Contains(payload.Query, "search(query:"):
			fixture = `{"data":{"search":{"issueCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}}`
		case strings.Contains(payload.Query, "nodes(ids:$ids)"):
			ids := payload.Variables["ids"].([]any)
			sizes = append(sizes, len(ids))
			fixture = `{"data":{"nodes":[` + strings.TrimSuffix(strings.Repeat("null,", len(ids)), ",") + `]}}`
		default:
			t.Fatalf("unexpected query: %s", payload.Query)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fixture))}, nil
	})})
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "PR_" + strconv.Itoa(i)
	}
	result, err := client.Fetch(context.Background(), testToken, "octocat", ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 2 || sizes[0] != 100 || sizes[1] != 1 || len(result.InaccessibleIDs) != 101 {
		t.Fatalf("tracked IDs were not batched: sizes=%v inaccessible=%d", sizes, len(result.InaccessibleIDs))
	}
}

func TestCompleteReviewAndCheckConnectionsPaginates(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		var payload graphRequest
		_ = json.NewDecoder(req.Body).Decode(&payload)
		var data string
		switch {
		case strings.Contains(payload.Query, "reviewRequests(first:100,after:$after)"):
			data = `{"data":{"node":{"reviewRequests":{"nodes":[{"requestedReviewer":{"__typename":"User","login":"octocat"}}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}`
		case strings.Contains(payload.Query, "contexts(first:100,after:$after)"):
			data = `{"data":{"node":{"statusCheckRollup":{"contexts":{"nodes":[{"__typename":"StatusContext","state":"SUCCESS"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}}}`
		default:
			t.Fatalf("unexpected pagination query: %s", payload.Query)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(data))}, nil
	})})
	var raw rawPR
	if err := json.Unmarshal([]byte(`{"id":"PR_X","reviewRequests":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":"review-cursor"}},"latestReviews":{"nodes":[],"pageInfo":{"hasNextPage":false}},"statusCheckRollup":{"contexts":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":"check-cursor"}}}}`), &raw); err != nil {
		t.Fatal(err)
	}
	complete, err := client.completeReviewPages(context.Background(), testToken, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(complete.ReviewRequests.Nodes) != 1 || !complete.StatusCheckRollup.Contexts.PageInfo.HasNextPage && len(complete.StatusCheckRollup.Contexts.Nodes) != 1 {
		t.Fatalf("connections were not completed: %+v", complete)
	}
	if got := normalizeCI(complete.StatusCheckRollup); got != model.CIPassed {
		t.Fatalf("paginated CI = %s", got)
	}
}

func TestForbiddenWithSecondaryLimitMessageIsRateLimited(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":"You have exceeded a secondary rate limit."}`))}, nil
	})})
	err := client.request(context.Background(), testToken, "query{viewer{login}}", nil, &map[string]any{})
	var rate *RateLimitError
	if !errors.As(err, &rate) || rate.RetryAfter != time.Minute {
		t.Fatalf("secondary limit was not classified safely: %v", err)
	}
}

func TestClientDoesNotFollowRedirectsWithAuthorization(t *testing.T) {
	calls := 0
	client := NewClient(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		h := make(http.Header)
		h.Set("Location", "https://attacker.example/collect")
		return &http.Response{StatusCode: http.StatusFound, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	err := client.request(context.Background(), testToken, "query{viewer{login}}", nil, &map[string]any{})
	if !errors.Is(err, ErrUnavailable) || calls != 1 {
		t.Fatalf("request followed an API redirect: err=%v calls=%d", err, calls)
	}
}

func TestNormalizeCIBoundariesAndPRURLValidation(t *testing.T) {
	for _, tc := range []struct {
		name, status, conclusion string
		want                     model.CI
	}{
		{"pass", "COMPLETED", "SUCCESS", model.CIPassed},
		{"skipped", "COMPLETED", "SKIPPED", model.CIPassed},
		{"fail", "COMPLETED", "FAILURE", model.CINotPassed},
		{"unknown", "COMPLETED", "", model.CINotAvailable},
		{"future conclusion", "COMPLETED", "FUTURE_VALUE", model.CINotAvailable},
		{"running", "IN_PROGRESS", "", model.CIRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var conclusion any
			if tc.conclusion != "" {
				conclusion = tc.conclusion
			}
			payload, _ := json.Marshal(map[string]any{"__typename": "CheckRun", "status": tc.status, "conclusion": conclusion})
			rollup := &struct {
				State    string `json:"state"`
				Contexts struct {
					Nodes    []json.RawMessage `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"contexts"`
			}{}
			rollup.Contexts.Nodes = []json.RawMessage{payload}
			if got := normalizeCI(rollup); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
	if got := normalizeCI(nil); got != model.CINotAvailable {
		t.Fatalf("missing checks = %s", got)
	}
	if !validPRURL("https://github.com/acme/project/pull/42") {
		t.Fatal("valid PR URL rejected")
	}
	for _, raw := range []string{"https://github.com.evil/acme/project/pull/42", "https://github.com/acme/project/issues/42", "https://github.com/acme/project/pull/42?x=1", "javascript:alert(1)"} {
		if validPRURL(raw) {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}

func prFixture(id string, number int, title, author, state, ci string, requested, closed bool) string {
	closedAt := `null`
	if closed {
		closedAt = `"2026-10-20T10:00:00Z"`
	}
	reviewReq := `[]`
	if requested {
		reviewReq = `[{"requestedReviewer":{"__typename":"User","login":"octocat"}}]`
	}
	if state == "OPEN" && id == "PR_B" {
		reviewReq = `[{"requestedReviewer":{"__typename":"Team","slug":"engineering","organization":{"login":"acme"}}}]`
	}
	status, conclusion, statusState := "COMPLETED", ci, ci
	if ci == "IN_PROGRESS" {
		status = ci
		conclusion = ""
	}
	if ci == "UNKNOWN" {
		status = "COMPLETED"
		conclusion = ""
	}
	return `{"__typename":"PullRequest","id":"` + id + `","number":` + strconv.Itoa(number) + `,"title":"` + title + `","url":"https://github.com/acme/project/pull/` + strconv.Itoa(number) + `","createdAt":"2026-10-01T10:00:00Z","updatedAt":"2026-10-20T09:00:00Z","closedAt":` + closedAt + `,"merged":` + strconv.FormatBool(closed && ci == "FAILURE") + `,"author":{"login":"` + author + `"},"reviewDecision":"APPROVED","latestReviews":{"nodes":[{"author":{"login":"reviewer"},"state":"APPROVED"}],"pageInfo":{"hasNextPage":false,"endCursor":null}},"reviewRequests":{"nodes":` + reviewReq + `,"pageInfo":{"hasNextPage":false,"endCursor":null}},"statusCheckRollup":{"state":"` + statusState + `","contexts":{"nodes":[{"__typename":"CheckRun","status":"` + status + `","conclusion":` + nullableString(conclusion) + `}],"pageInfo":{"hasNextPage":false,"endCursor":null}}},"repository":{"nameWithOwner":"acme/project"}}`
}

func nullableString(s string) string {
	if s == "" {
		return "null"
	}
	return `"` + s + `"`
}
