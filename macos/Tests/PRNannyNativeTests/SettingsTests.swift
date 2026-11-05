import MenubarKit
import Testing

@testable import PRNannyNative

@Test func saveKeepsTheConnectedAccountAndMethod() {
    let current = AppSettings(authMethod: "pat", account: "fixture-account")
    var edited = current
    edited.authMethod = "gh"
    edited.account = "someone-else"
    edited.intervalSeconds = 300
    let saved = SettingsRules.preferencesToSave(edited, current: current)
    #expect(saved.authMethod == "pat")
    #expect(saved.account == "fixture-account")
    #expect(saved.intervalSeconds == 300)
}

@Test func offersOnlyIntervalsTheHelperAccepts() {
    #expect(RefreshInterval.all.map(\.seconds) == [30, 60, 120, 300])
    #expect(RefreshInterval.all.map(\.label) == ["30 seconds", "60 seconds", "2 minutes", "5 minutes"])
}

@Test func eventTogglesCoverEveryPreference() {
    #expect(
        EventToggle.all.map(\.label) == [
            "CI status changes", "Approval changes", "Review requests", "Pull request merged", "Pull request closed",
        ])
    var events = EventPreferences()
    for toggle in EventToggle.all { events[keyPath: toggle.keyPath] = false }
    #expect(
        events == EventPreferences(ci: false, approval: false, reviewRequested: false, merged: false, closed: false))
}

@Test func masterToggleMirrorsIntoHelperSettings() {
    let current = AppSettings(notifications: true)
    #expect(SettingsRules.withNotifications(false, current: current).notifications == false)
    #expect(SettingsRules.withNotifications(false, current: current).events == current.events)
}

@Test func authMethodsAreLabelled() {
    #expect(AuthMethod.allCases.map(\.label) == ["GitHub CLI", "Personal access token"])
    #expect(AuthMethod(rawValue: "pat") == .pat)
}

@Test func notificationsAreOffByDefault() {
    #expect(NotificationPreferences() == NotificationPreferences(enabled: false, soundEnabled: false))
}

@Test func mapsHelperNotificationsWithDedupeIdentifiers() {
    let entry = HelperNotification(
        id: "0123456789abcdef0123456789abcdef", prId: "PR_fixture", event: "merged",
        title: "Pull request merged", subtitle: "sample/repo #42", body: "Fixture title")
    let notification = NotificationMapping.notification(for: entry)
    #expect(notification.identifier == entry.id)
    #expect(notification.threadIdentifier == "PR_fixture")
    #expect(notification.title == "Pull request merged")
    #expect(notification.body == "sample/repo #42\nFixture title")
    var bare = entry
    bare.subtitle = ""
    #expect(NotificationMapping.notification(for: bare).body == "Fixture title")
}
