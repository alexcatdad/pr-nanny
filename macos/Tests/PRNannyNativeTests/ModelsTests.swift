import Foundation
import Testing

@testable import PRNannyNative

private let snapshotJSON = """
    {"settings":{"version":1,"authMethod":"pat","account":"fixture-account","intervalSeconds":120,
     "layout":"tabs","notifications":true,"events":{"ci":true,"approval":false,"reviewRequested":true,
     "merged":true,"closed":false},"launchAtLogin":false},
     "pullRequests":[{"id":"PR_fixture","number":42,"title":"<img src=x onerror=alert(1)> **bold** fixture",
      "repository":"sample/private-repo","url":"https://github.com/sample/private-repo/pull/42",
      "createdAt":"2026-10-20T02:00:00Z","updatedAt":"2026-10-20T03:00:00.123456789Z",
      "closedAt":"2026-10-20T04:59:30.5+00:00","merged":true,"approved":false,"ci":"some_new_api_state",
      "authored":true,"reviewRequested":false,"bell":true}],
     "connection":"connected","message":"","lastRefresh":"2026-10-20T04:58:00Z","refreshing":false,"degraded":false}
    """

@Test func decodesHelperSnapshot() throws {
    let snapshot = try GoTime.decoder().decode(Snapshot.self, from: Data(snapshotJSON.utf8))
    #expect(snapshot.settings.authMethod == "pat")
    #expect(snapshot.settings.intervalSeconds == 120)
    #expect(snapshot.settings.events.approval == false)
    let pr = try #require(snapshot.pullRequests.first)
    // API text is kept verbatim; the view renders it with Text(verbatim:).
    #expect(pr.title == "<img src=x onerror=alert(1)> **bold** fixture")
    #expect(pr.ci == .notAvailable)
    #expect(pr.createdAt == fixtureNow.addingTimeInterval(-3 * 3600))
    #expect(abs(pr.updatedAt.timeIntervalSince(fixtureNow.addingTimeInterval(-7200)) - 0.123456789) < 0.000_01)
    #expect(pr.closedAt == fixtureNow.addingTimeInterval(-29.5))
    #expect(snapshot.lastRefresh == fixtureNow.addingTimeInterval(-120))
}

@Test func decodesGoNullsAndOmittedFields() throws {
    let json = """
        {"settings":{"version":1,"authMethod":"gh","account":"","intervalSeconds":60,"layout":"all",
         "notifications":true,"events":{"ci":true,"approval":true,"reviewRequested":true,"merged":true,"closed":true},
         "launchAtLogin":true},"pullRequests":null,"connection":"disconnected","message":"","refreshing":false,"degraded":false}
        """
    let snapshot = try GoTime.decoder().decode(Snapshot.self, from: Data(json.utf8))
    #expect(snapshot.pullRequests.isEmpty)
    #expect(snapshot.lastRefresh == nil)
}

@Test func encodesSettingsWithExactlyTheGoFields() throws {
    let data = try JSONEncoder().encode(AppSettings())
    let object = try #require(try JSONSerialization.jsonObject(with: data) as? [String: Any])
    #expect(
        Set(object.keys) == [
            "version", "authMethod", "account", "intervalSeconds", "layout", "notifications", "events",
            "launchAtLogin",
        ])
    let events = try #require(object["events"] as? [String: Any])
    #expect(Set(events.keys) == ["ci", "approval", "reviewRequested", "merged", "closed"])
}

@Test func rejectsMalformedTimestamps() {
    #expect(GoTime.parse("2026-10-20T02:00:00Z") != nil)
    #expect(GoTime.parse("2026-10-20T02:00:00.Z") == nil)
    #expect(GoTime.parse("2026-10-20T02:00:00.123") == nil)
    #expect(GoTime.parse("yesterday") == nil)
}

@Test func decodesNotificationBatches() throws {
    let json = """
        {"notifications":[{"id":"0123456789abcdef0123456789abcdef","prId":"PR_fixture","event":"ci",
         "title":"CI status changed","subtitle":"sample/repo #42","body":"Fixture title"}],"cursor":"7","missed":true}
        """
    let batch = try GoTime.decoder().decode(NotificationBatch.self, from: Data(json.utf8))
    #expect(batch.cursor == "7")
    #expect(batch.missed)
    #expect(batch.notifications.first?.prId == "PR_fixture")
    let empty = try GoTime.decoder().decode(
        NotificationBatch.self, from: Data(#"{"notifications":null,"cursor":""}"#.utf8))
    #expect(empty.notifications.isEmpty)
}
