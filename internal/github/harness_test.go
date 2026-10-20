package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub is a deterministic synthetic GraphQL server. It models just enough
// of GitHub's search, node and organization behavior for permission and cost
// tests, counts every request, and never sees real credentials.
type fakeGitHub struct {
	mu    sync.Mutex
	login string
	// Organization and team world.
	orgs  []string
	teams map[string][]string // org -> team slugs the viewer belongs to
	// Permission modes. "" means allowed.
	orgListMode string            // "forbidden", "http403", "http401", "scopes"
	teamMode    map[string]string // org -> "forbidden", "notfound", "http403"
	searchMode  map[string]string // "org/team" -> "forbidden"
	// Pull request world.
	prs []fakePR
	// Optional secondary rate limit: after this many points in a cycle, 403.
	throttleAfterPoints int
	// Accounting.
	stats fakeStats
}

type fakePR struct {
	id       string
	number   int
	author   string
	closed   bool
	merged   bool
	userReq  bool
	teamReqs []string // "org/slug"
}

type fakeStats struct {
	Requests  int
	Bytes     int
	Points    int
	ByKind    map[string]int
	Throttled int
}

const simulatedRTT = 150 * time.Millisecond // documented assumption for sequential latency estimates

func newFake(login string) *fakeGitHub {
	return &fakeGitHub{login: login, teams: map[string][]string{}, teamMode: map[string]string{}, searchMode: map[string]string{}, stats: fakeStats{ByKind: map[string]int{}}}
}

// addTeams creates one org with n teams named t0..tn-1 and returns their org/slug names.
func (f *fakeGitHub) addTeams(org string, n int) []string {
	f.orgs = append(f.orgs, org)
	var names []string
	for i := 0; i < n; i++ {
		slug := "t" + strconv.Itoa(i)
		f.teams[org] = append(f.teams[org], slug)
		names = append(names, org+"/"+slug)
	}
	return names
}

func (f *fakeGitHub) resetStats() {
	f.mu.Lock()
	f.stats = fakeStats{ByKind: map[string]int{}}
	f.mu.Unlock()
}

func (f *fakeGitHub) snapshot() fakeStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.stats
	out.ByKind = map[string]int{}
	for k, v := range f.stats.ByKind {
		out.ByKind[k] = v
	}
	return out
}

func (f *fakeGitHub) client(t testing.TB) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(srv.Close)
	c := NewClient(srv.Client())
	c.Endpoint = srv.URL
	return c, srv
}

// pointCost mirrors GitHub's documented GraphQL node-limit formula: add the
// requests needed for every connection at every level, divide by 100, round,
// minimum 1. It is an estimate; the production client does not request
// rateLimit{cost}.
func pointCost(requests int) int {
	p := (requests + 50) / 100
	if p < 1 {
		return 1
	}
	return p
}

func (f *fakeGitHub) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var payload graphRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	q := payload.Query
	kind, points := "other", 1
	switch {
	case q == `query{viewer{login}}`:
		kind = "viewer"
	case strings.Contains(q, "organizations(first:"):
		kind = "orgs"
	case strings.Contains(q, "organization(login:$org)"):
		kind = "teams"
	case strings.Contains(q, "search(query:"):
		if strings.Contains(q, "latestReviews") {
			kind = "search"
			points = pointCost(1 + 3*100)
		} else {
			kind = "search-ids"
			points = pointCost(1)
		}
	case strings.Contains(q, "nodes(ids:"):
		kind = "tracked"
		ids, _ := payload.Variables["ids"].([]any)
		points = pointCost(1 + 3*len(ids))
	case strings.Contains(q, "node(id:"):
		kind = "page"
	}
	f.stats.Requests++
	f.stats.ByKind[kind]++
	f.stats.Points += points
	if f.throttleAfterPoints > 0 && f.stats.Points > f.throttleAfterPoints {
		f.stats.Throttled++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"You have exceeded a secondary rate limit."}`)
		return
	}
	body, status := f.respond(kind, payload)
	f.stats.Bytes += len(body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func gqlErr(kind string, path ...any) string {
	return `{"data":null,"errors":[{"type":"` + kind + `","path":` + jsonOf(path) + `,"message":"synthetic"}]}`
}

func (f *fakeGitHub) respond(kind string, p graphRequest) (string, int) {
	empty := map[string]any{"hasNextPage": false, "endCursor": nil}
	switch kind {
	case "viewer":
		return `{"data":{"viewer":{"login":` + jsonOf(f.login) + `}}}`, 200
	case "orgs":
		switch f.orgListMode {
		case "http401":
			return `{"message":"Bad credentials"}`, 401
		case "http403":
			return `{"message":"Resource protected by organization SAML enforcement."}`, 403
		case "forbidden":
			return gqlErr("FORBIDDEN", "viewer", "organizations"), 200
		case "scopes":
			return gqlErr("INSUFFICIENT_SCOPES", "viewer", "organizations"), 200
		}
		var nodes []map[string]any
		for _, o := range f.orgs {
			nodes = append(nodes, map[string]any{"login": o})
		}
		return jsonOf(map[string]any{"data": map[string]any{"viewer": map[string]any{"login": f.login, "organizations": map[string]any{"nodes": nodes, "pageInfo": empty}}}}), 200
	case "teams":
		org, _ := p.Variables["org"].(string)
		switch f.teamMode[org] {
		case "forbidden":
			return gqlErr("FORBIDDEN", "organization", "teams"), 200
		case "notfound":
			return gqlErr("NOT_FOUND", "organization"), 200
		case "http403":
			return `{"message":"forbidden"}`, 403
		}
		var nodes []map[string]any
		for _, s := range f.teams[org] {
			nodes = append(nodes, map[string]any{"slug": s})
		}
		return jsonOf(map[string]any{"data": map[string]any{"organization": map[string]any{"teams": map[string]any{"nodes": nodes, "pageInfo": empty}}}}), 200
	case "search", "search-ids":
		search, _ := p.Variables["search"].(string)
		if team := search[strings.LastIndex(search, ":")+1:]; strings.Contains(search, "team-review-requested:") && f.searchMode[team] == "forbidden" {
			return gqlErr("FORBIDDEN", "search"), 200
		}
		matches := f.searchMatches(search)
		offset := 0
		if after, _ := p.Variables["after"].(string); after != "" {
			offset, _ = strconv.Atoi(strings.TrimPrefix(after, "o"))
		}
		end := offset + 100
		if end > len(matches) {
			end = len(matches)
		}
		var nodes []json.RawMessage
		for _, pr := range matches[offset:end] {
			if kind == "search-ids" {
				nodes = append(nodes, json.RawMessage(jsonOf(map[string]any{"__typename": "PullRequest", "id": pr.id})))
			} else {
				nodes = append(nodes, json.RawMessage(f.prJSON(pr)))
			}
		}
		info := map[string]any{"hasNextPage": end < len(matches), "endCursor": nil}
		if end < len(matches) {
			info["endCursor"] = "o" + strconv.Itoa(end)
		}
		if nodes == nil {
			nodes = []json.RawMessage{}
		}
		return jsonOf(map[string]any{"data": map[string]any{"search": map[string]any{"issueCount": len(matches), "pageInfo": info, "nodes": nodes}}}), 200
	case "tracked":
		ids, _ := p.Variables["ids"].([]any)
		var nodes []json.RawMessage
		var errs []map[string]any
		for i, raw := range ids {
			id, _ := raw.(string)
			found := false
			for _, pr := range f.prs {
				if pr.id == id {
					nodes = append(nodes, json.RawMessage(f.prJSON(pr)))
					found = true
				}
			}
			if !found {
				nodes = append(nodes, json.RawMessage("null"))
				errs = append(errs, map[string]any{"type": "NOT_FOUND", "path": []any{"nodes", i}})
			}
		}
		out := map[string]any{"data": map[string]any{"nodes": nodes}}
		if len(errs) != 0 {
			out["errors"] = errs
		}
		return jsonOf(out), 200
	}
	return `{"data":null,"errors":[{"type":"UNEXPECTED"}]}`, 200
}

func (f *fakeGitHub) searchMatches(search string) []fakePR {
	var out []fakePR
	for _, pr := range f.prs {
		if pr.closed {
			continue
		}
		switch {
		case strings.Contains(search, "author:"):
			if strings.EqualFold(pr.author, strings.TrimPrefix(search[strings.Index(search, "author:"):], "author:")) {
				out = append(out, pr)
			}
		case strings.Contains(search, "user-review-requested:"):
			if pr.userReq {
				out = append(out, pr)
			}
		case strings.Contains(search, "team-review-requested:"):
			team := search[strings.Index(search, "team-review-requested:")+len("team-review-requested:"):]
			for _, t := range pr.teamReqs {
				if strings.EqualFold(t, team) {
					out = append(out, pr)
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].number > out[j].number })
	return out
}

func (f *fakeGitHub) prJSON(pr fakePR) string {
	var reqs []map[string]any
	if pr.userReq {
		reqs = append(reqs, map[string]any{"id": "RRU_" + pr.id, "requestedReviewer": map[string]any{"__typename": "User", "login": f.login}})
	}
	for _, t := range pr.teamReqs {
		org, slug, _ := strings.Cut(t, "/")
		reqs = append(reqs, map[string]any{"id": "RRT_" + pr.id + "_" + slug, "requestedReviewer": map[string]any{"__typename": "Team", "slug": slug, "organization": map[string]any{"login": org}}})
	}
	if reqs == nil {
		reqs = []map[string]any{}
	}
	var closedAt any
	if pr.closed {
		closedAt = "2026-10-20T10:00:00Z"
	}
	empty := map[string]any{"hasNextPage": false, "endCursor": nil}
	return jsonOf(map[string]any{
		"__typename": "PullRequest", "id": pr.id, "number": pr.number, "title": "Synthetic " + pr.id,
		"url":       "https://github.com/acme/project/pull/" + strconv.Itoa(pr.number),
		"createdAt": "2026-10-01T10:00:00Z", "updatedAt": fmt.Sprintf("2026-10-20T09:%02d:00Z", pr.number%60), "closedAt": closedAt, "merged": pr.merged,
		"author":         map[string]any{"login": pr.author},
		"latestReviews":  map[string]any{"nodes": []any{}, "pageInfo": empty},
		"reviewRequests": map[string]any{"nodes": reqs, "pageInfo": empty},
		"statusCheckRollup": map[string]any{"state": "SUCCESS", "contexts": map[string]any{
			"nodes":    []any{map[string]any{"__typename": "CheckRun", "status": "COMPLETED", "conclusion": "SUCCESS"}},
			"pageInfo": empty}},
		"repository": map[string]any{"nameWithOwner": "acme/project"},
	})
}

func (f *fakeGitHub) fetch(t *testing.T, c *Client, tracked ...string) ([]string, []string, error) {
	t.Helper()
	res, err := c.Fetch(context.Background(), fixtureToken, f.login, tracked)
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	for _, pr := range res.PullRequests {
		ids = append(ids, pr.ID)
	}
	sort.Strings(ids)
	return ids, res.Warnings, nil
}

type serverError struct{}

func (serverError) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusBadGateway)
}

type constantBody string

func (b constantBody) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, string(b))
}
