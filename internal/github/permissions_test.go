package github

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

// permissionWorld has an authored PR, a direct review request and a PR that is
// only visible through a team review request.
func permissionWorld() *fakeGitHub {
	f := newFake("octocat")
	f.addTeams("acme", 1)
	f.prs = []fakePR{
		{id: "PR_AUTH", number: 1, author: "octocat"},
		{id: "PR_DIRECT", number: 2, author: "other", userReq: true},
		{id: "PR_TEAM", number: 3, author: "other", teamReqs: []string{"acme/t0"}},
	}
	return f
}

func clockAt(now *time.Time) func() time.Time { return func() time.Time { return *now } }

func TestPermissionFullAccessFindsEverythingWithoutWarning(t *testing.T) {
	f := permissionWorld()
	c, _ := f.client(t)
	ids, warnings, err := f.fetch(t, c)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	if !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT", "PR_TEAM"}) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestPermissionNoOrganizationMembershipIsNotDegraded(t *testing.T) {
	f := permissionWorld()
	f.orgs, f.teams = nil, map[string][]string{}
	f.prs = f.prs[:2]
	c, _ := f.client(t)
	ids, warnings, err := f.fetch(t, c)
	if err != nil || len(warnings) != 0 || len(ids) != 2 {
		t.Fatalf("ids=%v warnings=%v err=%v", ids, warnings, err)
	}
}

func TestPermissionOrgVisibleButTeamListingForbiddenDegradesOnlyTeamReview(t *testing.T) {
	f := permissionWorld()
	f.teamMode["acme"] = "forbidden"
	c, _ := f.client(t)
	ids, warnings, err := f.fetch(t, c)
	if err != nil {
		t.Fatalf("team listing failure must not abort the sync: %v", err)
	}
	if !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT"}) {
		t.Fatalf("authored and direct review must still work: %v", ids)
	}
	if len(warnings) != 1 || warnings[0] != model.WarningTeamReview {
		t.Fatalf("warnings = %v", warnings)
	}
	for _, w := range warnings {
		if strings.Contains(w, "acme") {
			t.Fatal("warning leaked an organization name")
		}
	}
}

func TestPermissionOneDeniedOrgDoesNotHideOtherOrgTeams(t *testing.T) {
	f := permissionWorld()
	f.addTeams("globex", 1)
	f.teamMode["globex"] = "notfound" // org policy hides the org
	c, _ := f.client(t)
	ids, warnings, err := f.fetch(t, c)
	if err != nil || len(warnings) != 1 || !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT", "PR_TEAM"}) {
		t.Fatalf("ids=%v warnings=%v err=%v", ids, warnings, err)
	}
}

func TestPermissionRestrictedOrgScopesKeepRepoAccessWorking(t *testing.T) {
	for _, mode := range []string{"http403", "forbidden", "scopes"} {
		f := permissionWorld()
		f.orgListMode = mode
		c, _ := f.client(t)
		ids, warnings, err := f.fetch(t, c)
		if err != nil || len(warnings) != 1 || !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT"}) {
			t.Fatalf("%s: ids=%v warnings=%v err=%v", mode, ids, warnings, err)
		}
	}
}

func TestPermissionBadCredentialsStillFailAsAuth(t *testing.T) {
	f := permissionWorld()
	f.orgListMode = "http401"
	c, _ := f.client(t)
	if _, _, err := f.fetch(t, c); !errors.Is(err, ErrAuth) {
		t.Fatalf("401 must remain ErrAuth, got %v", err)
	}
}

func TestPermissionOtherFailuresStillAbortTheCycle(t *testing.T) {
	// A 5xx during team discovery is transient, not a permission problem.
	f := permissionWorld()
	c, srv := f.client(t)
	srv.Config.Handler = serverError{}
	if _, _, err := f.fetch(t, c); !errors.Is(err, ErrServer) {
		t.Fatalf("5xx must abort the cycle, got %v", err)
	}
}

func TestPermissionAccountMismatchStillRejectedWhenOrgsDenied(t *testing.T) {
	f := permissionWorld()
	f.orgListMode = "http403"
	f.login = "someone-else"
	c, _ := f.client(t)
	if _, err := c.Fetch(context.Background(), fixtureToken, "octocat", nil); !errors.Is(err, ErrAuth) {
		t.Fatalf("token for another account must be ErrAuth, got %v", err)
	}
}

func TestPermissionRevokedMidSessionUsesBoundedStaleCache(t *testing.T) {
	f := permissionWorld()
	c, _ := f.client(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	c.now = clockAt(&now)
	ids, warnings, err := f.fetch(t, c)
	if err != nil || len(warnings) != 0 || len(ids) != 3 {
		t.Fatalf("baseline: %v %v %v", ids, warnings, err)
	}
	// Revocation inside the TTL is invisible: the warm cache is used.
	f.orgListMode = "http403"
	f.resetStats()
	if _, warnings, _ = f.fetch(t, c); len(warnings) != 0 || f.snapshot().ByKind["orgs"] != 0 {
		t.Fatalf("warm cache should not re-discover: %v %v", warnings, f.snapshot().ByKind)
	}
	// After the TTL discovery is denied; the last good list stands in with a warning.
	now = now.Add(teamCacheTTL + time.Second)
	ids, warnings, err = f.fetch(t, c)
	if err != nil || len(warnings) != 1 || !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT", "PR_TEAM"}) {
		t.Fatalf("stale fallback: ids=%v warnings=%v err=%v", ids, warnings, err)
	}
	// The denied result is negative-cached for one TTL: no hammering.
	f.resetStats()
	if _, warnings, _ = f.fetch(t, c); len(warnings) != 1 || f.snapshot().ByKind["orgs"] != 0 {
		t.Fatalf("degraded state should be cached: %v %v", warnings, f.snapshot().ByKind)
	}
	// Past the grace window the stale list is dropped (treated as revoked).
	now = now.Add(teamStaleGrace + time.Minute)
	ids, warnings, err = f.fetch(t, c)
	if err != nil || len(warnings) != 1 || !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT"}) {
		t.Fatalf("after grace: ids=%v warnings=%v err=%v", ids, warnings, err)
	}
}

func TestPermissionRestoredRecoversFullTeamReview(t *testing.T) {
	f := permissionWorld()
	f.orgListMode = "http403"
	c, _ := f.client(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	c.now = clockAt(&now)
	if ids, warnings, _ := f.fetch(t, c); len(warnings) != 1 || len(ids) != 2 {
		t.Fatalf("degraded: %v %v", ids, warnings)
	}
	f.orgListMode = ""
	now = now.Add(teamCacheTTL + time.Second)
	ids, warnings, err := f.fetch(t, c)
	if err != nil || len(warnings) != 0 || len(ids) != 3 {
		t.Fatalf("recovered: ids=%v warnings=%v err=%v", ids, warnings, err)
	}
}

func TestPermissionBadCredentialsClearTeamCache(t *testing.T) {
	f := permissionWorld()
	c, _ := f.client(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	c.now = clockAt(&now)
	if _, _, err := f.fetch(t, c); err != nil {
		t.Fatal(err)
	}
	f.orgListMode = "http401"
	now = now.Add(teamCacheTTL + time.Second)
	if _, _, err := f.fetch(t, c); !errors.Is(err, ErrAuth) {
		t.Fatalf("got %v", err)
	}
	f.orgListMode = "http403"
	ids, warnings, _ := f.fetch(t, c)
	if len(warnings) != 1 || len(ids) != 2 {
		t.Fatalf("revoked credentials must not leave a usable stale list: %v %v", ids, warnings)
	}
}

func TestPermissionTeamSearchForbiddenDegradesIndependently(t *testing.T) {
	f := permissionWorld()
	f.addTeams("globex", 1)
	f.prs = append(f.prs, fakePR{id: "PR_G", number: 4, author: "other", teamReqs: []string{"globex/t0"}})
	f.searchMode["acme/t0"] = "forbidden"
	c, _ := f.client(t)
	ids, warnings, err := f.fetch(t, c)
	if err != nil || len(warnings) != 1 || !reflect.DeepEqual(ids, []string{"PR_AUTH", "PR_DIRECT", "PR_G"}) {
		t.Fatalf("ids=%v warnings=%v err=%v", ids, warnings, err)
	}
}

func TestPermissionTrackedTeamPRKeepsUnverifiedRequestIDsWhenDegraded(t *testing.T) {
	f := permissionWorld()
	f.orgListMode = "http403"
	c, _ := f.client(t)
	res, err := c.Fetch(context.Background(), fixtureToken, "octocat", []string{"PR_TEAM"})
	if err != nil {
		t.Fatal(err)
	}
	var got *model.PullRequest
	for i := range res.PullRequests {
		if res.PullRequests[i].ID == "PR_TEAM" {
			got = &res.PullRequests[i]
		}
	}
	if got == nil || got.ReviewRequested || !reflect.DeepEqual(got.UnverifiedTeamRequestIDs, []string{"RRT_PR_TEAM_t0"}) {
		t.Fatalf("tracked team PR = %+v", got)
	}
}

func TestPermissionNonPermissionGraphQLErrorsStillFail(t *testing.T) {
	f := permissionWorld()
	c, srv := f.client(t)
	srv.Config.Handler = constantBody(`{"data":null,"errors":[{"type":"INTERNAL","path":["viewer"]}]}`)
	if _, _, err := f.fetch(t, c); !errors.Is(err, ErrResponse) {
		t.Fatalf("got %v", err)
	}
}
