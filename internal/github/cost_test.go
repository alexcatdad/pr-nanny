package github

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// costWorld builds a deterministic world with nTeams teams. Each team has
// perTeam open review-requested PRs; every fifth PR is also requested from the
// next team (overlap) and, for team 0, also directly. Authored and closed PRs
// are fixed. It returns the world, the expected set of open PR IDs the
// monitor must discover, and the tracked (closed) IDs.
func costWorld(nTeams, perTeam, authored int) (*fakeGitHub, []string, []string) {
	f := newFake("octocat")
	names := f.addTeams("acme", nTeams)
	number := 0
	next := func() int { number++; return number }
	want := map[string]bool{}
	for i := 0; i < authored; i++ {
		n := next()
		id := "PR_A" + strconv.Itoa(n)
		f.prs = append(f.prs, fakePR{id: id, number: n, author: "octocat"})
		want[id] = true
	}
	for i := 0; i < 3; i++ {
		n := next()
		id := "PR_D" + strconv.Itoa(n)
		f.prs = append(f.prs, fakePR{id: id, number: n, author: "other", userReq: true})
		want[id] = true
	}
	for ti, team := range names {
		for j := 0; j < perTeam; j++ {
			n := next()
			id := "PR_T" + strconv.Itoa(n)
			pr := fakePR{id: id, number: n, author: "other", teamReqs: []string{team}}
			if j%5 == 0 && ti+1 < len(names) {
				pr.teamReqs = append(pr.teamReqs, names[ti+1])
			}
			if j%5 == 1 && ti == 0 {
				pr.userReq = true
			}
			f.prs = append(f.prs, pr)
			want[id] = true
		}
	}
	var tracked []string
	for i := 0; i < 5; i++ {
		n := next()
		id := "PR_C" + strconv.Itoa(n)
		f.prs = append(f.prs, fakePR{id: id, number: n, author: "octocat", closed: true, merged: i%2 == 0})
		tracked = append(tracked, id)
	}
	var ids []string
	for id := range want {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return f, ids, tracked
}

type costRow struct {
	name                  string
	requests, points      int
	bytes                 int
	latency, wall         time.Duration
	searches, ids, tracks int
}

func measure(t *testing.T, f *fakeGitHub, c *Client, tracked []string, wantOpen []string, name string) costRow {
	t.Helper()
	f.resetStats()
	start := time.Now()
	res, err := c.Fetch(context.Background(), fixtureToken, f.login, tracked)
	wall := time.Since(start)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var open, closed []string
	for _, pr := range res.PullRequests {
		if pr.ClosedAt != nil {
			closed = append(closed, pr.ID)
		} else {
			open = append(open, pr.ID)
		}
	}
	sort.Strings(open)
	sort.Strings(closed)
	wantClosed := append([]string(nil), tracked...)
	sort.Strings(wantClosed)
	if !reflect.DeepEqual(open, wantOpen) || !reflect.DeepEqual(closed, wantClosed) {
		t.Fatalf("%s: incomplete discovery: open %d/%d closed %d/%d", name, len(open), len(wantOpen), len(closed), len(wantClosed))
	}
	// Ground truth: every returned PR carries exactly the request state the world
	// defines, so grouping or batching cannot change notification semantics.
	truth := map[string]fakePR{}
	for _, pr := range f.prs {
		truth[pr.id] = pr
	}
	for _, pr := range res.PullRequests {
		w := truth[pr.ID]
		wantReq := w.userReq || len(w.teamReqs) > 0
		var wantIDs []string
		if w.userReq {
			wantIDs = append(wantIDs, "RRU_"+w.id)
		}
		for _, team := range w.teamReqs {
			_, slug, _ := strings.Cut(team, "/")
			wantIDs = append(wantIDs, "RRT_"+w.id+"_"+slug)
		}
		sort.Strings(wantIDs)
		idsMatch := len(wantIDs) == 0 && len(pr.ReviewRequestIDs) == 0 || reflect.DeepEqual(pr.ReviewRequestIDs, wantIDs)
		if pr.ReviewRequested != wantReq || pr.Authored != (w.author == f.login) || !idsMatch {
			t.Fatalf("%s: %s has wrong request state: %+v want requested=%v ids=%v", name, pr.ID, pr, wantReq, wantIDs)
		}
	}
	s := f.snapshot()
	return costRow{name: name, requests: s.Requests, points: s.Points, bytes: s.Bytes, latency: time.Duration(s.Requests) * simulatedRTT, wall: wall,
		searches: s.ByKind["search"], ids: s.ByKind["search-ids"], tracks: s.ByKind["tracked"]}
}

func (r costRow) String() string {
	return fmt.Sprintf("%-34s requests=%-3d (search=%d ids=%d tracked=%d) points~%-4d bytes=%-8d latency~%-6s wall=%s", r.name, r.requests, r.searches, r.ids, r.tracks, r.points, r.bytes, r.latency, r.wall.Round(time.Microsecond))
}

// TestCostMatrix records GraphQL cost per polling cycle and asserts complete
// discovery for every scenario. Run with -v to print the table that
// docs/API-COST.md is built from.
func TestCostMatrix(t *testing.T) {
	for _, teams := range []int{0, 5, 25, 50} {
		f, want, tracked := costWorld(teams, 4, 3)
		c, _ := f.client(t)
		now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		c.now = clockAt(&now)
		cold := measure(t, f, c, tracked, want, fmt.Sprintf("teams=%d cold cache", teams))
		warm := measure(t, f, c, tracked, want, fmt.Sprintf("teams=%d warm cache", teams))
		now = now.Add(teamCacheTTL + time.Second)
		expired := measure(t, f, c, tracked, want, fmt.Sprintf("teams=%d expired cache", teams))
		t.Log(cold)
		t.Log(warm)
		t.Log(expired)
		if expired.requests != cold.requests {
			t.Fatalf("expired cache should cost the same as cold: %d vs %d", expired.requests, cold.requests)
		}
	}
}

// TestCostPaginationAndOverlap uses result sets larger than one page: a team
// with 250 PRs (3 pages) and authored PRs beyond 100, with overlaps.
func TestCostPaginationAndOverlap(t *testing.T) {
	f, want, tracked := costWorld(2, 250, 130)
	c, _ := f.client(t)
	row := measure(t, f, c, tracked, want, "teams=2 paginated (250/team,130 own)")
	t.Log(row)
	if len(want) < 600 {
		t.Fatalf("world too small: %d", len(want))
	}
}

// TestCostSecondaryRateLimit shows that a secondary limit hit mid-cycle aborts
// the whole cycle with a safe retry hint and no partial result.
func TestCostSecondaryRateLimit(t *testing.T) {
	f, _, tracked := costWorld(50, 4, 3)
	f.throttleAfterPoints = 60
	c, _ := f.client(t)
	f.resetStats()
	res, err := c.Fetch(context.Background(), fixtureToken, f.login, tracked)
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != time.Minute || len(res.PullRequests) != 0 {
		t.Fatalf("err=%v prs=%d", err, len(res.PullRequests))
	}
	s := f.snapshot()
	t.Logf("secondary limit: aborted after %d requests / ~%d points, %d throttled, retry in %s, partial results returned: %d", s.Requests, s.Points, s.Throttled, rl.RetryAfter, len(res.PullRequests))
	if s.Throttled != 1 {
		t.Fatalf("client kept calling after being throttled: %d", s.Throttled)
	}
}

func BenchmarkFetchWarmCache(b *testing.B) {
	for _, teams := range []int{0, 5, 25, 50} {
		b.Run(fmt.Sprintf("teams=%d", teams), func(b *testing.B) {
			f, _, tracked := costWorld(teams, 4, 3)
			c, _ := f.client(b)
			if _, err := c.Fetch(context.Background(), fixtureToken, f.login, tracked); err != nil {
				b.Fatal(err)
			}
			f.resetStats()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := c.Fetch(context.Background(), fixtureToken, f.login, tracked); err != nil {
					b.Fatal(err)
				}
			}
			s := f.snapshot()
			b.ReportMetric(float64(s.Requests)/float64(b.N), "requests/op")
			b.ReportMetric(float64(s.Points)/float64(b.N), "points/op")
			b.ReportMetric(float64(s.Bytes)/float64(b.N), "bytes/op")
		})
	}
}
