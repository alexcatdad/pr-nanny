package app

import "testing"

func TestValidPRURL(t *testing.T) {
	valid := []string{
		"https://github.com/acme/project/pull/1",
		"https://github.com/a-b/repo.name/pull/987654",
	}
	for _, raw := range valid {
		t.Run(raw, func(t *testing.T) {
			if !ValidPRURL(raw) {
				t.Fatalf("ValidPRURL(%q) = false", raw)
			}
		})
	}
	invalid := []string{
		"http://github.com/acme/project/pull/1",
		"https://evil.example/acme/project/pull/1",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"https://github.com\\@evil.example/acme/project/pull/1",
		"https://github.com/acme/project/pull/%31",
		"https://github.com/acme/project%2fpull/1",
		"https://user@github.com/acme/project/pull/1",
		"https://github.com:443/acme/project/pull/1",
		"https://github.com/acme/project/pull/1?next=https://evil.example",
		"https://github.com/acme/project/pull/1#fragment",
		" https://github.com/acme/project/pull/1",
		"https://github.com/acme/project/pull/0",
		"https://github.com/acme/project/issues/1",
	}
	for _, raw := range invalid {
		t.Run(raw, func(t *testing.T) {
			if ValidPRURL(raw) {
				t.Fatalf("ValidPRURL(%q) = true", raw)
			}
		})
	}
}
