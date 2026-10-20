package app

import (
	"net/url"
	"regexp"
	"strings"
)

var prPath = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9_.-]+/pull/[1-9][0-9]*$`)

// ValidPRURL accepts a canonical GitHub.com pull request only. No user-info,
// ports, escaped paths, fragments, queries, controls, redirects or other schemes.
func ValidPRURL(raw string) bool {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\\\r\n\t") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.RawPath == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && prPath.MatchString(u.Path)
}
