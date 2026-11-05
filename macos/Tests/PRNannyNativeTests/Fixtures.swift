import Foundation

@testable import PRNannyNative

let fixtureNow = Date(timeIntervalSince1970: 1_792_472_400)  // 2026-10-20T05:00:00Z

func fixturePR(
    _ id: String, number: Int = 1, updatedAgo: TimeInterval = 600, closedAgo: TimeInterval? = nil,
    authored: Bool = false, reviewRequested: Bool = false, bell: Bool = false
) -> PullRequest {
    PullRequest(
        id: id, number: number, title: "Fixture \(id)", repository: "sample/repo",
        url: "https://github.com/sample/repo/pull/\(number)",
        createdAt: fixtureNow.addingTimeInterval(-3 * 3600),
        updatedAt: fixtureNow.addingTimeInterval(-updatedAgo),
        closedAt: closedAgo.map { fixtureNow.addingTimeInterval(-$0) },
        authored: authored, reviewRequested: reviewRequested, bell: bell)
}

func fixtureSnapshot(connection: String = "connected", message: String = "", prs: [PullRequest] = []) -> Snapshot {
    Snapshot(
        settings: AppSettings(account: "fixture-account"), pullRequests: prs, connection: connection,
        message: message, lastRefresh: fixtureNow.addingTimeInterval(-120))
}
