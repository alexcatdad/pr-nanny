import Testing

@testable import PRNannyNative

// Mirrors internal/app/url_test.go so Swift is never looser than Go.
@Test(arguments: [
    "https://github.com/acme/project/pull/1",
    "https://github.com/a-b/repo.name/pull/987654",
])
func acceptsCanonicalPullRequestURLs(_ raw: String) {
    #expect(PRURL.isValid(raw))
    #expect(PRURL.url(raw)?.absoluteString == raw)
}

@Test(arguments: [
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
    "https://github.com/acme/project/pull/1 ",
    "https://github.com/acme/project/pull/0",
    "https://github.com/acme/project/issues/1",
    "https://github.com/acme/../pull/1",
    "https://github.com/acme/./pull/1",
    "https://github.com.evil.example/acme/project/pull/1",
    "https://github.com/acme/project/pull/1/files",
    "https://github.com/-acme/project/pull/1",
    "https://github.com/acme/proj\u{0}ect/pull/1",
    "https://github.com/acmé/project/pull/1",
    "",
])
func rejectsEverythingElse(_ raw: String) {
    #expect(!PRURL.isValid(raw))
    #expect(PRURL.url(raw) == nil)
}

@Test func rejectsOverlongURLs() {
    let raw = "https://github.com/acme/" + String(repeating: "a", count: 2048) + "/pull/1"
    #expect(!PRURL.isValid(raw))
}
