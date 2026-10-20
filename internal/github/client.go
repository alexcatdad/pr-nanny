// Package github implements read-only GitHub.com PR discovery.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

const (
	endpoint       = "https://api.github.com/graphql"
	maxPageCount   = 20
	maxTrackedIDs  = 10000
	trackedBatch   = 100
	maxConnections = 100
	maxResponseLen = 8 << 20
)

var (
	ErrAuth         = &Failure{kind: "auth", message: "GitHub authentication failed"}
	ErrRateLimited  = &Failure{kind: "rate_limit", message: "GitHub rate limit reached", retry: time.Minute}
	ErrUnavailable  = &Failure{kind: "offline", message: "GitHub is temporarily unavailable"}
	ErrServer       = &Failure{kind: "server", message: "GitHub is temporarily unavailable"}
	ErrResponse     = &Failure{kind: "server", message: "GitHub returned an unexpected response"}
	ErrInvalidInput = errors.New("invalid GitHub request")
)

// Failure exposes only safe category metadata to the polling layer.
type Failure struct {
	kind, message string
	retry         time.Duration
}

func (e *Failure) Error() string             { return e.message }
func (e *Failure) Kind() string              { return e.kind }
func (e *Failure) RetryDelay() time.Duration { return e.retry }

// RateLimitError carries a safe retry hint without retaining response bodies.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string             { return ErrRateLimited.Error() }
func (e *RateLimitError) Is(target error) bool      { return target == ErrRateLimited }
func (e *RateLimitError) Kind() string              { return "rate_limit" }
func (e *RateLimitError) RetryDelay() time.Duration { return e.RetryAfter }

type Client struct {
	HTTP     *http.Client
	Endpoint string // injectable for deterministic tests; production uses GitHub.com
	teamMu   sync.Mutex
	teams    map[string]teamCacheEntry
}

type teamCacheEntry struct {
	values  []string
	expires time.Time
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 25 * time.Second}
	}
	return &Client{HTTP: httpClient, Endpoint: endpoint}
}

type graphRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Type string `json:"type"`
		Path []any  `json:"path"`
	} `json:"errors"`
}

func (c *Client) Fetch(ctx context.Context, token, account string, trackedIDs []string) (model.FetchResult, error) {
	if !validToken(token) || !validLogin(account) || len(trackedIDs) > maxTrackedIDs {
		return model.FetchResult{}, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return model.FetchResult{}, err
	}
	base := c.Endpoint
	if base == "" {
		base = endpoint
	}
	u, err := url.Parse(base)
	if err != nil || !(u.Scheme == "https" && u.Host == "api.github.com" || u.Scheme == "http" && isLoopbackHost(u.Hostname())) || u.User != nil {
		return model.FetchResult{}, ErrInvalidInput
	}
	prMap := make(map[string]model.PullRequest)
	queries := []string{
		"is:pr is:open author:" + account,
		"is:pr is:open user-review-requested:" + account,
	}
	teams, err := c.viewerTeams(ctx, token, account)
	if err != nil {
		return model.FetchResult{}, err
	}
	for _, team := range teams {
		queries = append(queries, "is:pr is:open team-review-requested:"+team)
	}
	for _, query := range queries {
		if err := c.fetchSearch(ctx, token, account, query, teams, prMap); err != nil {
			return model.FetchResult{}, err
		}
	}
	result := model.FetchResult{}
	if len(trackedIDs) != 0 {
		for start := 0; start < len(trackedIDs); start += trackedBatch {
			end := start + trackedBatch
			if end > len(trackedIDs) {
				end = len(trackedIDs)
			}
			tracked, inaccessible, err := c.fetchTracked(ctx, token, account, teams, trackedIDs[start:end])
			if err != nil {
				return model.FetchResult{}, err
			}
			for id, pr := range tracked {
				prMap[id] = pr
			}
			result.InaccessibleIDs = append(result.InaccessibleIDs, inaccessible...)
		}
	}
	for _, pr := range prMap {
		result.PullRequests = append(result.PullRequests, pr)
	}
	sort.Slice(result.PullRequests, func(i, j int) bool {
		if result.PullRequests[i].UpdatedAt.Equal(result.PullRequests[j].UpdatedAt) {
			return result.PullRequests[i].ID < result.PullRequests[j].ID
		}
		return result.PullRequests[i].UpdatedAt.After(result.PullRequests[j].UpdatedAt)
	})
	sort.Strings(result.InaccessibleIDs)
	return result, nil
}

func (c *Client) viewerTeams(ctx context.Context, token, login string) ([]string, error) {
	key := strings.ToLower(login)
	c.teamMu.Lock()
	if cached, ok := c.teams[key]; ok && time.Now().Before(cached.expires) {
		values := append([]string(nil), cached.values...)
		c.teamMu.Unlock()
		return values, nil
	}
	c.teamMu.Unlock()
	const orgQuery = `query($after:String){viewer{login organizations(first:100,after:$after){nodes{login} pageInfo{hasNextPage endCursor}}}}`
	const teamQuery = `query($login:String!,$org:String!,$after:String){organization(login:$org){teams(first:100,after:$after,userLogins:[$login]){nodes{slug} pageInfo{hasNextPage endCursor}}}}`
	var teams []string
	var cursor any
	for page := 0; page < maxConnections; page++ {
		var orgData struct {
			Viewer struct {
				Login         string `json:"login"`
				Organizations struct {
					Nodes []struct {
						Login string `json:"login"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"organizations"`
			} `json:"viewer"`
		}
		if err := c.request(ctx, token, orgQuery, map[string]any{"after": cursor}, &orgData); err != nil {
			return nil, err
		}
		if !strings.EqualFold(orgData.Viewer.Login, login) {
			return nil, ErrAuth
		}
		for _, org := range orgData.Viewer.Organizations.Nodes {
			if !validLogin(org.Login) {
				return nil, ErrResponse
			}
			var teamCursor any
			for teamPage := 0; teamPage < maxConnections; teamPage++ {
				var teamData struct {
					Organization *struct {
						Teams struct {
							Nodes []struct {
								Slug string `json:"slug"`
							} `json:"nodes"`
							PageInfo struct {
								HasNextPage bool   `json:"hasNextPage"`
								EndCursor   string `json:"endCursor"`
							} `json:"pageInfo"`
						} `json:"teams"`
					} `json:"organization"`
				}
				if err := c.request(ctx, token, teamQuery, map[string]any{"login": login, "org": org.Login, "after": teamCursor}, &teamData); err != nil {
					return nil, err
				}
				if teamData.Organization == nil {
					return nil, ErrResponse
				}
				for _, team := range teamData.Organization.Teams.Nodes {
					if !validSlug(team.Slug) {
						return nil, ErrResponse
					}
					teams = append(teams, org.Login+"/"+team.Slug)
				}
				if !teamData.Organization.Teams.PageInfo.HasNextPage {
					break
				}
				if teamPage == maxConnections-1 || teamData.Organization.Teams.PageInfo.EndCursor == "" {
					return nil, ErrResponse
				}
				teamCursor = teamData.Organization.Teams.PageInfo.EndCursor
			}
		}
		if !orgData.Viewer.Organizations.PageInfo.HasNextPage {
			break
		}
		if page == maxConnections-1 || orgData.Viewer.Organizations.PageInfo.EndCursor == "" {
			return nil, ErrResponse
		}
		cursor = orgData.Viewer.Organizations.PageInfo.EndCursor
	}
	sort.Strings(teams)
	c.teamMu.Lock()
	if c.teams == nil {
		c.teams = make(map[string]teamCacheEntry)
	}
	c.teams[key] = teamCacheEntry{values: append([]string(nil), teams...), expires: time.Now().Add(5 * time.Minute)}
	c.teamMu.Unlock()
	return teams, nil
}

const pullFragment = `nodes{__typename ... on PullRequest{id number title url createdAt updatedAt closedAt merged author{login} reviewDecision latestReviews(first:100){nodes{author{login} state} pageInfo{hasNextPage endCursor}} reviewRequests(first:100){nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug organization{login}}}} pageInfo{hasNextPage endCursor}} statusCheckRollup{state contexts(first:100){nodes{__typename ... on CheckRun{status conclusion} ... on StatusContext{state}} pageInfo{hasNextPage endCursor}}} repository{nameWithOwner}}}`

func (c *Client) fetchSearch(ctx context.Context, token, login, search string, teams []string, dst map[string]model.PullRequest) error {
	const query = `query($search:String!,$after:String){search(query:$search,type:ISSUE,first:100,after:$after){issueCount pageInfo{hasNextPage endCursor} ` + pullFragment + `}}`
	var cursor any
	for page := 0; page < maxPageCount; page++ {
		var data struct {
			Search struct {
				IssueCount int `json:"issueCount"`
				PageInfo   struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []rawPR `json:"nodes"`
			} `json:"search"`
		}
		if err := c.request(ctx, token, query, map[string]any{"search": search, "after": cursor}, &data); err != nil {
			return err
		}
		for _, raw := range data.Search.Nodes {
			if raw.Type != "PullRequest" {
				continue
			}
			raw, err := c.completeReviewPages(ctx, token, raw)
			if err != nil {
				return err
			}
			pr, err := raw.normalize(login, teams)
			if err != nil {
				return err
			}
			if old, ok := dst[pr.ID]; ok {
				pr.ReviewRequested = old.ReviewRequested || pr.ReviewRequested
			}
			dst[pr.ID] = pr
		}
		if !data.Search.PageInfo.HasNextPage {
			return nil
		}
		if page == maxPageCount-1 || data.Search.PageInfo.EndCursor == "" || data.Search.IssueCount >= 1000 && page >= 9 {
			return ErrResponse
		}
		cursor = data.Search.PageInfo.EndCursor
	}
	return ErrResponse
}

func (c *Client) fetchTracked(ctx context.Context, token, login string, teams, ids []string) (map[string]model.PullRequest, []string, error) {
	for _, id := range ids {
		if !validNodeID(id) {
			return nil, nil, ErrInvalidInput
		}
	}
	const query = `query($ids:[ID!]!){nodes(ids:$ids){__typename ... on PullRequest{id number title url createdAt updatedAt closedAt merged author{login} reviewDecision latestReviews(first:100){nodes{author{login} state} pageInfo{hasNextPage endCursor}} reviewRequests(first:100){nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug organization{login}}}} pageInfo{hasNextPage endCursor}} statusCheckRollup{state contexts(first:100){nodes{__typename ... on CheckRun{status conclusion} ... on StatusContext{state}} pageInfo{hasNextPage endCursor}}} repository{nameWithOwner}}}}`
	var data struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := c.request(ctx, token, query, map[string]any{"ids": ids}, &data); err != nil {
		return nil, nil, err
	}
	tracked := make(map[string]model.PullRequest)
	missing := make([]string, 0)
	seen := make(map[string]bool)
	for _, rawNode := range data.Nodes {
		if bytes.Equal(bytes.TrimSpace(rawNode), []byte("null")) {
			continue
		}
		var raw rawPR
		if err := json.Unmarshal(rawNode, &raw); err != nil {
			return nil, nil, ErrResponse
		}
		if raw.Type != "PullRequest" {
			continue
		}
		completed, err := c.completeReviewPages(ctx, token, raw)
		if err != nil {
			return nil, nil, err
		}
		raw = completed
		pr, err := raw.normalize(login, teams)
		if err != nil {
			return nil, nil, err
		}
		tracked[pr.ID] = pr
		seen[pr.ID] = true
	}
	// A null Node can mean deleted, inaccessible, or no longer a PR. Preserve its
	// previous baseline; callers must not infer a closure or removed request.
	for _, id := range ids {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	return tracked, missing, nil
}

func (c *Client) request(ctx context.Context, token, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(graphRequest{Query: query, Variables: variables})
	if err != nil {
		return ErrInvalidInput
	}
	requestURL := c.Endpoint
	if requestURL == "" {
		requestURL = endpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return ErrInvalidInput
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden && isRateLimitResponse(resp) {
		return rateError(resp)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuth
	}
	if resp.StatusCode >= 500 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return ErrServer
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return ErrUnavailable
	}
	var envelope graphEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseLen)).Decode(&envelope); err != nil {
		return ErrResponse
	}
	if len(envelope.Errors) != 0 {
		ciOnlyErrors := true
		for _, e := range envelope.Errors {
			if e.Type == "RATE_LIMITED" {
				return rateError(resp)
			}
			ciPath := false
			for _, part := range e.Path {
				if part == "statusCheckRollup" || part == "contexts" {
					ciPath = true
				}
			}
			ciOnlyErrors = ciOnlyErrors && ciPath
			if (e.Type == "UNAUTHORIZED" || e.Type == "FORBIDDEN") && !ciPath {
				return ErrAuth
			}
		}
		if !ciOnlyErrors {
			return ErrResponse
		}
	}
	if len(envelope.Data) == 0 || string(bytes.TrimSpace(envelope.Data)) == "null" {
		return ErrResponse
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return ErrResponse
	}
	if !validGraphShape(envelope.Data, query, len(envelope.Errors) != 0) {
		return ErrResponse
	}
	return nil
}

// GraphQL structs with zero values can make truncated successful responses
// look like an empty snapshot. Check required connection metadata before the
// operation-specific decoder consumes the response.
func validGraphShape(data json.RawMessage, query string, allowUnavailableCI bool) bool {
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil || root == nil || len(root) == 0 {
		return false
	}
	path := func(parts ...string) json.RawMessage {
		cur := data
		for _, key := range parts {
			var object map[string]json.RawMessage
			if json.Unmarshal(cur, &object) != nil {
				return nil
			}
			cur = object[key]
			if len(cur) == 0 {
				return nil
			}
		}
		return cur
	}
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		var value map[string]json.RawMessage
		if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" || json.Unmarshal(raw, &value) != nil {
			return nil
		}
		return value
	}
	connection := func(parts ...string) bool {
		conn := object(path(parts...))
		if conn == nil {
			return false
		}
		var nodes []json.RawMessage
		if json.Unmarshal(conn["nodes"], &nodes) != nil {
			return false
		}
		pi := object(conn["pageInfo"])
		if pi == nil {
			return false
		}
		var more bool
		return json.Unmarshal(pi["hasNextPage"], &more) == nil
	}
	switch {
	case strings.Contains(query, "organizations(first:"):
		return object(path("viewer")) != nil && connection("viewer", "organizations")
	case strings.Contains(query, "teams(first:"):
		return object(path("organization")) != nil && connection("organization", "teams")
	case strings.Contains(query, "search(query:"):
		if !connection("search") {
			return false
		}
		var count int
		return json.Unmarshal(object(path("search"))["issueCount"], &count) == nil && validPRConnectionNodes(path("search", "nodes"), allowUnavailableCI)
	case strings.Contains(query, "nodes(ids:"):
		var nodes []json.RawMessage
		return json.Unmarshal(path("nodes"), &nodes) == nil && validPRConnectionNodes(path("nodes"), allowUnavailableCI)
	case strings.Contains(query, "reviewRequests(first:100,after:"):
		return object(path("node")) != nil && connection("node", "reviewRequests")
	case strings.Contains(query, "latestReviews(first:100,after:"):
		return object(path("node")) != nil && connection("node", "latestReviews")
	case strings.Contains(query, "contexts(first:100,after:"):
		if object(path("node")) == nil {
			return false
		}
		raw := path("node", "statusCheckRollup")
		return allowUnavailableCI && len(raw) == 0 || len(raw) > 0 && (string(bytes.TrimSpace(raw)) == "null" || connection("node", "statusCheckRollup", "contexts"))
	}
	return false
}

func validPRConnectionNodes(raw json.RawMessage, allowUnavailableCI bool) bool {
	var nodes []json.RawMessage
	if json.Unmarshal(raw, &nodes) != nil {
		return false
	}
	for _, item := range nodes {
		if string(bytes.TrimSpace(item)) == "null" {
			continue
		}
		var pr map[string]json.RawMessage
		if json.Unmarshal(item, &pr) != nil {
			return false
		}
		var typename string
		if json.Unmarshal(pr["__typename"], &typename) != nil {
			return false
		}
		if typename != "PullRequest" {
			continue
		}
		for _, name := range []string{"latestReviews", "reviewRequests"} {
			conn := pr[name]
			if len(conn) == 0 {
				return false
			}
			var decoded map[string]json.RawMessage
			if json.Unmarshal(conn, &decoded) != nil || decoded == nil {
				return false
			}
			var childNodes []json.RawMessage
			if json.Unmarshal(decoded["nodes"], &childNodes) != nil {
				return false
			}
			var page map[string]json.RawMessage
			if json.Unmarshal(decoded["pageInfo"], &page) != nil || page == nil {
				return false
			}
			var more bool
			if json.Unmarshal(page["hasNextPage"], &more) != nil {
				return false
			}
		}
		ci := pr["statusCheckRollup"]
		if len(ci) == 0 && allowUnavailableCI {
			continue
		}
		if len(ci) == 0 {
			return false
		}
		if string(bytes.TrimSpace(ci)) != "null" {
			var roll map[string]json.RawMessage
			if json.Unmarshal(ci, &roll) != nil || roll == nil {
				return false
			}
			var contexts []json.RawMessage
			if json.Unmarshal(roll["contexts"], &contexts) != nil { // below: contexts is a connection, not its node list
				if !validRawConnection(roll["contexts"]) {
					return false
				}
			}
		}
	}
	return true
}

func validRawConnection(raw json.RawMessage) bool {
	var conn map[string]json.RawMessage
	if json.Unmarshal(raw, &conn) != nil || conn == nil {
		return false
	}
	var nodes []json.RawMessage
	if json.Unmarshal(conn["nodes"], &nodes) != nil {
		return false
	}
	var page map[string]json.RawMessage
	if json.Unmarshal(conn["pageInfo"], &page) != nil || page == nil {
		return false
	}
	var more bool
	return json.Unmarshal(page["hasNextPage"], &more) == nil
}

func (c *Client) httpClient() *http.Client {
	var client *http.Client
	if c.HTTP != nil {
		copy := *c.HTTP
		client = &copy
	} else {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

func rateError(resp *http.Response) error {
	delay := time.Minute
	if value := resp.Header.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 && seconds <= 86400 {
			delay = time.Duration(seconds) * time.Second
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			if d := time.Until(time.Unix(reset, 0)); d > delay && d <= 24*time.Hour {
				delay = d
			}
		}
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return &RateLimitError{RetryAfter: delay}
}

func isRateLimitResponse(resp *http.Response) bool {
	if resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.ToLower(string(body))
	return strings.Contains(message, "secondary rate limit") || strings.Contains(message, "rate limit exceeded")
}

type rawPR struct {
	Type      string  `json:"__typename"`
	ID        string  `json:"id"`
	Number    int     `json:"number"`
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	ClosedAt  *string `json:"closedAt"`
	Merged    bool    `json:"merged"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewDecision *string `json:"reviewDecision"`
	LatestReviews  struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
		} `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"latestReviews"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Type         string `json:"__typename"`
				Login        string `json:"login"`
				Slug         string `json:"slug"`
				Organization struct {
					Login string `json:"login"`
				} `json:"organization"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"reviewRequests"`
	StatusCheckRollup *struct {
		State    string `json:"state"`
		Contexts struct {
			Nodes    []json.RawMessage `json:"nodes"`
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"contexts"`
	} `json:"statusCheckRollup"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

func (c *Client) completeReviewPages(ctx context.Context, token string, raw rawPR) (rawPR, error) {
	const requestQuery = `query($id:ID!,$after:String){node(id:$id){... on PullRequest{reviewRequests(first:100,after:$after){nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug organization{login}}}} pageInfo{hasNextPage endCursor}}}}}`
	for page := 0; raw.ReviewRequests.PageInfo.HasNextPage; page++ {
		if page >= maxConnections || raw.ReviewRequests.PageInfo.EndCursor == "" {
			return rawPR{}, ErrResponse
		}
		var data struct {
			Node *struct {
				ReviewRequests struct {
					Nodes []struct {
						RequestedReviewer struct {
							Type         string `json:"__typename"`
							Login        string `json:"login"`
							Slug         string `json:"slug"`
							Organization struct {
								Login string `json:"login"`
							} `json:"organization"`
						} `json:"requestedReviewer"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"reviewRequests"`
			} `json:"node"`
		}
		if err := c.request(ctx, token, requestQuery, map[string]any{"id": raw.ID, "after": raw.ReviewRequests.PageInfo.EndCursor}, &data); err != nil {
			return rawPR{}, err
		}
		if data.Node == nil {
			return rawPR{}, ErrResponse
		}
		raw.ReviewRequests.Nodes = append(raw.ReviewRequests.Nodes, data.Node.ReviewRequests.Nodes...)
		raw.ReviewRequests.PageInfo = data.Node.ReviewRequests.PageInfo
	}
	const reviewsQuery = `query($id:ID!,$after:String){node(id:$id){... on PullRequest{latestReviews(first:100,after:$after){nodes{author{login} state} pageInfo{hasNextPage endCursor}}}}}`
	for page := 0; raw.LatestReviews.PageInfo.HasNextPage; page++ {
		if page >= maxConnections || raw.LatestReviews.PageInfo.EndCursor == "" {
			return rawPR{}, ErrResponse
		}
		var data struct {
			Node *struct {
				LatestReviews struct {
					Nodes []struct {
						Author struct {
							Login string `json:"login"`
						} `json:"author"`
						State string `json:"state"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"latestReviews"`
			} `json:"node"`
		}
		if err := c.request(ctx, token, reviewsQuery, map[string]any{"id": raw.ID, "after": raw.LatestReviews.PageInfo.EndCursor}, &data); err != nil {
			return rawPR{}, err
		}
		if data.Node == nil {
			return rawPR{}, ErrResponse
		}
		raw.LatestReviews.Nodes = append(raw.LatestReviews.Nodes, data.Node.LatestReviews.Nodes...)
		raw.LatestReviews.PageInfo = data.Node.LatestReviews.PageInfo
	}
	const contextsQuery = `query($id:ID!,$after:String){node(id:$id){... on PullRequest{statusCheckRollup{contexts(first:100,after:$after){nodes{__typename ... on CheckRun{status conclusion} ... on StatusContext{state}} pageInfo{hasNextPage endCursor}}}}}}`
	for page := 0; raw.StatusCheckRollup != nil && raw.StatusCheckRollup.Contexts.PageInfo.HasNextPage; page++ {
		if page >= maxConnections || raw.StatusCheckRollup.Contexts.PageInfo.EndCursor == "" {
			return rawPR{}, ErrResponse
		}
		var data struct {
			Node *struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes    []json.RawMessage `json:"nodes"`
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"node"`
		}
		if err := c.request(ctx, token, contextsQuery, map[string]any{"id": raw.ID, "after": raw.StatusCheckRollup.Contexts.PageInfo.EndCursor}, &data); err != nil {
			return rawPR{}, err
		}
		if data.Node == nil || data.Node.StatusCheckRollup == nil {
			raw.StatusCheckRollup = nil
			break
		}
		raw.StatusCheckRollup.Contexts.Nodes = append(raw.StatusCheckRollup.Contexts.Nodes, data.Node.StatusCheckRollup.Contexts.Nodes...)
		raw.StatusCheckRollup.Contexts.PageInfo = data.Node.StatusCheckRollup.Contexts.PageInfo
	}
	return raw, nil
}

func (r rawPR) normalize(login string, teams []string) (model.PullRequest, error) {
	created, err := time.Parse(time.RFC3339, r.CreatedAt)
	if err != nil {
		return model.PullRequest{}, ErrResponse
	}
	updated, err := time.Parse(time.RFC3339, r.UpdatedAt)
	if err != nil {
		return model.PullRequest{}, ErrResponse
	}
	var closed *time.Time
	if r.ClosedAt != nil {
		t, err := time.Parse(time.RFC3339, *r.ClosedAt)
		if err != nil {
			return model.PullRequest{}, ErrResponse
		}
		closed = &t
	}
	if !validNodeID(r.ID) || r.Number <= 0 || !validRepo(r.Repository.NameWithOwner) || !validPRURL(r.URL) || !validLogin(r.Author.Login) || r.LatestReviews.PageInfo.HasNextPage || r.ReviewRequests.PageInfo.HasNextPage {
		return model.PullRequest{}, ErrResponse
	}
	approved := false
	for _, review := range r.LatestReviews.Nodes {
		if strings.EqualFold(review.Author.Login, r.Author.Login) {
			continue
		}
		if review.State == "APPROVED" {
			approved = true
		}
	}
	reviewRequested := false
	for _, req := range r.ReviewRequests.Nodes {
		if strings.EqualFold(req.RequestedReviewer.Login, login) {
			reviewRequested = true
		}
		if req.RequestedReviewer.Type == "Team" {
			name := req.RequestedReviewer.Organization.Login + "/" + req.RequestedReviewer.Slug
			for _, team := range teams {
				if strings.EqualFold(team, name) {
					reviewRequested = true
					break
				}
			}
		}
	}
	pr := model.PullRequest{ID: r.ID, Number: r.Number, Title: r.Title, Repository: r.Repository.NameWithOwner, URL: r.URL, CreatedAt: created, UpdatedAt: updated, ClosedAt: closed, Merged: r.Merged, Approved: approved, CI: normalizeCI(r.StatusCheckRollup), Authored: strings.EqualFold(r.Author.Login, login), ReviewRequested: reviewRequested}
	return pr, nil
}

func normalizeCI(rollup *struct {
	State    string `json:"state"`
	Contexts struct {
		Nodes    []json.RawMessage `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"contexts"`
}) model.CI {
	if rollup == nil || rollup.Contexts.PageInfo.HasNextPage || len(rollup.Contexts.Nodes) == 0 {
		return model.CINotAvailable
	}
	failed, pending := false, false
	for _, raw := range rollup.Contexts.Nodes {
		var item struct {
			Type       string  `json:"__typename"`
			Status     string  `json:"status"`
			Conclusion *string `json:"conclusion"`
			State      string  `json:"state"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return model.CINotAvailable
		}
		switch item.Type {
		case "CheckRun":
			switch item.Status {
			case "COMPLETED":
				if item.Conclusion == nil {
					return model.CINotAvailable
				}
				switch *item.Conclusion {
				case "SUCCESS", "NEUTRAL", "SKIPPED":
				case "FAILURE", "STARTUP_FAILURE", "TIMED_OUT", "ACTION_REQUIRED", "CANCELLED", "STALE":
					failed = true
				default:
					return model.CINotAvailable
				}
			case "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED", "PENDING":
				pending = true
			default:
				return model.CINotAvailable
			}
		case "StatusContext":
			switch item.State {
			case "SUCCESS":
			case "PENDING", "EXPECTED":
				pending = true
			case "ERROR", "FAILURE":
				failed = true
			default:
				return model.CINotAvailable
			}
		default:
			return model.CINotAvailable
		}
	}
	if failed {
		return model.CINotPassed
	}
	if pending {
		return model.CIRunning
	}
	return model.CIPassed
}

func validPRURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "pull" || !validLogin(parts[0]) || !validRepoComponent(parts[1]) {
		return false
	}
	_, err = strconv.Atoi(parts[3])
	return err == nil && parts[3] != "0"
}

func validToken(s string) bool { return len(s) >= 20 && len(s) <= 300 && strings.TrimSpace(s) == s }
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func validLogin(s string) bool {
	if len(s) == 0 || len(s) > 39 || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") || strings.Contains(s, "--") {
		return false
	}
	return validSlug(s) && !strings.Contains(s, "/")
}
func validSlug(s string) bool {
	if s == "" || len(s) > 100 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func validRepo(s string) bool {
	p := strings.Split(s, "/")
	return len(p) == 2 && validLogin(p[0]) && validRepoComponent(p[1])
}

func validRepoComponent(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
func validNodeID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
