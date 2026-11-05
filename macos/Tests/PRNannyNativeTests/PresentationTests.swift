import Foundation
import MenubarKit
import Testing

@testable import PRNannyNative

@Test func usesShortFriendlyAgesAndClampsFutureTimestamps() {
    #expect(relativeAge(fixtureNow.addingTimeInterval(-3 * 3600), now: fixtureNow) == "3h")
    #expect(relativeAge(fixtureNow.addingTimeInterval(-2 * 86_400), now: fixtureNow) == "2d")
    #expect(relativeAge(fixtureNow.addingTimeInterval(60), now: fixtureNow) == "now")
    #expect(relativeAge(fixtureNow.addingTimeInterval(-59), now: fixtureNow) == "now")
    #expect(relativeAge(fixtureNow.addingTimeInterval(-60), now: fixtureNow) == "1m")
    #expect(relativeAge(fixtureNow.addingTimeInterval(-40 * 86_400), now: fixtureNow) == "1mo")
    #expect(relativeAge(fixtureNow.addingTimeInterval(-400 * 86_400), now: fixtureNow) == "1y")
}

@Test func labelsCIAndApprovalAsText() {
    #expect(CIState.passed.label == "Passed")
    #expect(CIState.notPassed.label == "Not passed")
    #expect(CIState.running.label == "Running")
    #expect(CIState.notAvailable.label == "Not available")
    var pr = fixturePR("a")
    #expect(pr.approvalLabel == "Not approved")
    pr.approved = true
    #expect(pr.approvalLabel == "Approved")
}

@Test func rowLabelsKeepOpenAndBellSeparate() {
    var pr = fixturePR("a", number: 42)
    pr.repository = "sample/private-repo"
    pr.title = "<script>x</script>"
    #expect(pr.accessibilityOpenLabel == "Open sample/private-repo pull request #42: <script>x</script>")
    #expect(pr.bellLabel == "Turn notifications on")
    pr.bell = true
    #expect(pr.bellLabel == "Turn notifications off")
    #expect(pr.repositoryParts.owner == "sample/")
    #expect(pr.repositoryParts.name == "private-repo")
    pr.repository = "noslash"
    #expect(pr.repositoryParts.owner == "")
    #expect(pr.repositoryParts.name == "noslash")
}

@Test func filtersCategoriesNewestFirst() {
    let prs = [
        fixturePR("old-authored", updatedAgo: 900, authored: true),
        fixturePR("new-authored", updatedAgo: 60, authored: true),
        fixturePR("review", reviewRequested: true),
        fixturePR("both", authored: true, reviewRequested: true),
        fixturePR("closed-authored", closedAgo: 10, authored: true),
    ]
    #expect(PRCategory.authored.filter(prs, now: fixtureNow).map(\.id) == ["new-authored", "both", "old-authored"])
    #expect(PRCategory.review.filter(prs, now: fixtureNow).map(\.id) == ["review", "both"])
    #expect(PRCategory.recent.filter(prs, now: fixtureNow).map(\.id) == ["closed-authored"])
}

@Test func recentClosuresExpireFromTheClosureTimestamp() {
    let prs = [
        fixturePR("just", closedAgo: 59), fixturePR("expired", closedAgo: 60), fixturePR("future", closedAgo: -5),
    ]
    #expect(PRCategory.recent.filter(prs, now: fixtureNow).map(\.id) == ["just"])
    #expect(PRCategory.recent.filter(prs, now: fixtureNow.addingTimeInterval(2)).isEmpty)
}

@Test func duplicateRowsShareOneBellState() {
    // A PR both authored and review-requested appears twice; both rows read the same model.
    let pr = fixturePR("dup", authored: true, reviewRequested: true, bell: true)
    let authored = PRCategory.authored.filter([pr], now: fixtureNow)
    let review = PRCategory.review.filter([pr], now: fixtureNow)
    #expect(authored.first?.bell == true && review.first?.bell == true)
}

@Test func layoutFallsBackToAll() {
    #expect(PanelLayout(settings: AppSettings(layout: "tabs")) == .tabs)
    #expect(PanelLayout(settings: AppSettings(layout: "grid")) == .all)
    #expect(PanelLayout.allCases.map(\.label) == ["Show all", "Tabs"])
    #expect(PRCategory.allCases.map(\.tabLabel) == ["Authored", "Review", "Recent"])
}

@Test func describesConnections() {
    #expect(ConnectionPresentation.connectionLine(fixtureSnapshot()) == "Connected as fixture-account")
    #expect(
        ConnectionPresentation.connectionLine(fixtureSnapshot(connection: "offline"))
            == "Offline · showing saved pull requests")
    #expect(
        ConnectionPresentation.connectionLine(fixtureSnapshot(connection: "authentication_error"))
            == "Authentication expired")
    #expect(ConnectionPresentation.connectionLine(fixtureSnapshot(connection: "mystery")) == "Not connected")
    for state in ["disconnected", "missing_auth", "auth_error", "authentication_error"] {
        #expect(ConnectionPresentation.authRequired(state))
    }
    #expect(!ConnectionPresentation.authRequired("offline"))
    #expect(ConnectionPresentation.needsReconnect("authentication_error"))
    #expect(!ConnectionPresentation.needsReconnect("disconnected"))
}

@Test func showsRefreshAgeAsJustNowOrElapsed() {
    var snapshot = fixtureSnapshot()
    #expect(ConnectionPresentation.lastRefresh(snapshot, now: fixtureNow) == "Updated 2m ago")
    snapshot.lastRefresh = fixtureNow.addingTimeInterval(-5)
    #expect(ConnectionPresentation.lastRefresh(snapshot, now: fixtureNow) == "Updated just now")
    snapshot.lastRefresh = nil
    #expect(ConnectionPresentation.lastRefresh(snapshot, now: fixtureNow) == "")
}

@Test func bannerShowsHelperMessagesAndOfflineState() {
    #expect(!ConnectionPresentation.showsBanner(fixtureSnapshot()))
    let degraded = fixtureSnapshot(message: "Team review requests could not be checked.")
    #expect(ConnectionPresentation.showsBanner(degraded))
    #expect(ConnectionPresentation.statusMessage(degraded) == "Team review requests could not be checked.")
    let offline = fixtureSnapshot(connection: "offline")
    #expect(ConnectionPresentation.showsBanner(offline))
    #expect(ConnectionPresentation.statusMessage(offline).hasPrefix("You are offline."))
}

@Test func trayStatusBadgesErrorsAndAuthRequired() {
    let ok = ConnectionPresentation.status(
        helper: .running, snapshot: fixtureSnapshot(prs: [fixturePR("a")]), now: fixtureNow)
    #expect(ok.tone == .ok)
    #expect(!ok.tone.needsAttention)
    #expect(ok.word == "1 open pull request")
    #expect(TrayAccessibility.label(appName: "PR Nanny", statusWord: ok.word) == "PR Nanny, 1 open pull request")

    let auth = ConnectionPresentation.status(helper: .running, snapshot: fixtureSnapshot(connection: "disconnected"))
    #expect(auth.tone == .warning && auth.word == "Sign-in needed")

    let error = ConnectionPresentation.status(helper: .running, snapshot: fixtureSnapshot(connection: "error"))
    #expect(error.tone == .error && error.tone.needsAttention)

    let unavailable = ConnectionPresentation.status(
        helper: .unavailable("Helper stopped."), snapshot: fixtureSnapshot())
    #expect(unavailable.tone == .error && unavailable.word == "Unavailable" && unavailable.detail == "Helper stopped.")

    #expect(ConnectionPresentation.status(helper: .alreadyRunning, snapshot: nil).word == "Already running")
    #expect(ConnectionPresentation.status(helper: .starting, snapshot: nil).word == "Starting")
}

@Test func countsPullRequests() {
    #expect(ConnectionPresentation.countLabel(1) == "1 pull request")
    #expect(ConnectionPresentation.countLabel(3) == "3 pull requests")
}
