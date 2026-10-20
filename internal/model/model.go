// Package model defines the small, non-secret contract shared by the monitor and UI.
package model

import "time"

type CI string

const (
	CIPassed       CI = "passed"
	CINotPassed    CI = "not_passed"
	CIRunning      CI = "running"
	CINotAvailable CI = "not_available"
)

type PullRequest struct {
	ID              string     `json:"id"`
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	Repository      string     `json:"repository"`
	URL             string     `json:"url"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	ClosedAt        *time.Time `json:"closedAt,omitempty"`
	Merged          bool       `json:"merged"`
	Approved        bool       `json:"approved"`
	CI              CI         `json:"ci"`
	Authored        bool       `json:"authored"`
	ReviewRequested bool       `json:"reviewRequested"`
	// Request node IDs detect removed-and-recreated requests between polling cycles.
	// They never cross the renderer boundary.
	ReviewRequestIDs []string `json:"-"`
	Bell             bool     `json:"bell"`
}
type EventPreferences struct {
	CI              bool `json:"ci"`
	Approval        bool `json:"approval"`
	ReviewRequested bool `json:"reviewRequested"`
	Merged          bool `json:"merged"`
	Closed          bool `json:"closed"`
}
type Settings struct {
	Version         int              `json:"version"`
	AuthMethod      string           `json:"authMethod"`
	Account         string           `json:"account"`
	IntervalSeconds int              `json:"intervalSeconds"`
	Layout          string           `json:"layout"`
	Notifications   bool             `json:"notifications"`
	Events          EventPreferences `json:"events"`
	LaunchAtLogin   bool             `json:"launchAtLogin"`
}

func DefaultSettings() Settings {
	return Settings{Version: 1, AuthMethod: "gh", IntervalSeconds: 60, Layout: "all", Notifications: true, Events: EventPreferences{true, true, true, true, true}, LaunchAtLogin: true}
}

type Snapshot struct {
	Settings     Settings      `json:"settings"`
	PullRequests []PullRequest `json:"pullRequests"`
	Connection   string        `json:"connection"`
	Message      string        `json:"message"`
	LastRefresh  *time.Time    `json:"lastRefresh,omitempty"`
	Refreshing   bool          `json:"refreshing"`
}

// FetchResult is a complete successful fetch. Inaccessible IDs retain their prior baseline.
type FetchResult struct {
	PullRequests    []PullRequest
	InaccessibleIDs []string
}
