import AppKit
import Foundation
import MenubarKit
import SwiftUI
import Testing
import UserNotifications

@testable import PRNanny
@testable import PRNannyNative

/// `MenuBarExtra` can propose a minimal window height. A scroll view bounded
/// only by `.frame(maxHeight:)` then collapses to the panel padding and the
/// list shows about one row; these tests host the panel the same way.
@MainActor
@Suite struct PanelLayoutTests {
    /// Well above the collapsed height (about 24pt) and one or two rows.
    static let minimumListHeight: CGFloat = 300

    static func pullRequests(count: Int, now: Date = Date()) -> [PullRequest] {
        (0..<count).map { $0 + 1 }.map { index in
            PullRequest(
                id: "PR_fixture_\(index)", number: index, title: "Fixture pull request \(index)",
                repository: "sample/repo-\(index % 4)", url: "https://github.com/sample/repo/pull/\(index)",
                createdAt: now.addingTimeInterval(-Double(index) * 3600),
                updatedAt: now.addingTimeInterval(-Double(index) * 600),
                approved: index % 3 == 0, ci: index % 2 == 0 ? .passed : .running,
                authored: index <= 15, reviewRequested: index > 15, bell: index % 5 == 0)
        }
    }

    static func model(layout: PanelLayout, count: Int = 25) -> AppModel {
        let model = AppModel(notifier: MenubarNotifier(center: FakeNotificationCenter()))
        var settings = AppSettings(account: "fixture-account")
        settings.layout = layout.rawValue
        model.showForTesting(
            Snapshot(
                settings: settings, pullRequests: pullRequests(count: count), connection: "connected",
                message: "", lastRefresh: Date().addingTimeInterval(-120)))
        return model
    }

    /// Hosts a view, lets SwiftUI settle its measured heights, and returns the controller.
    static func host<V: View>(_ view: V) -> NSHostingController<V> {
        let controller = NSHostingController(rootView: view)
        controller.view.frame = NSRect(x: 0, y: 0, width: MenubarMetrics.panelWidth, height: 600)
        for _ in 0..<5 {
            controller.view.layoutSubtreeIfNeeded()
            RunLoop.main.run(until: Date().addingTimeInterval(0.05))
        }
        return controller
    }

    @Test(arguments: PanelLayout.allCases)
    func listKeepsItsHeightWhenTheWindowProposesNone(layout: PanelLayout) throws {
        let model = Self.model(layout: layout)
        let empty = Self.host(PanelView().environmentObject(Self.model(layout: layout, count: 0)))
        let full = Self.host(PanelView().environmentObject(model))
        let emptyHeight = empty.sizeThatFits(in: CGSize(width: MenubarMetrics.panelWidth, height: 0)).height
        let size = full.sizeThatFits(in: CGSize(width: MenubarMetrics.panelWidth, height: 0))
        // The list alone is the difference between 25 PRs and none.
        #expect(size.height - emptyHeight > Self.minimumListHeight, "panel \(size.height), no PRs \(emptyHeight)")
        #expect(size.width == MenubarMetrics.panelWidth)
        try Self.render(PanelView().environmentObject(model), size: size, name: "panel-\(layout.rawValue)")
    }

    @Test func rendersSettings() throws {
        let model = Self.model(layout: .all)
        let controller = Self.host(SettingsView().environmentObject(model))
        let size = controller.sizeThatFits(in: CGSize(width: MenubarMetrics.settingsWidth, height: 0))
        #expect(size.height > 300)
        try Self.render(SettingsView().environmentObject(model), size: size, name: "settings")
    }

    /// Writes a PNG into `$PR_NANNY_RENDER_ARTIFACTS` when it is set.
    static func render<V: View>(_ content: V, size: CGSize, name: String) throws {
        guard let directory = ProcessInfo.processInfo.environment["PR_NANNY_RENDER_ARTIFACTS"], !directory.isEmpty
        else { return }
        // The panel draws no background of its own (the menu-bar window does).
        let view = NSHostingView(rootView: content.background(Color(nsColor: .windowBackgroundColor)))
        let window = NSWindow(
            contentRect: NSRect(origin: .zero, size: size), styleMask: [.borderless], backing: .buffered, defer: false)
        window.contentView = view
        for _ in 0..<5 {
            view.layoutSubtreeIfNeeded()
            RunLoop.main.run(until: Date().addingTimeInterval(0.05))
        }
        let bitmap = try #require(view.bitmapImageRepForCachingDisplay(in: view.bounds))
        view.cacheDisplay(in: view.bounds, to: bitmap)
        let data = try #require(bitmap.representation(using: .png, properties: [:]))
        let url = URL(filePath: directory, directoryHint: .isDirectory).appending(path: "\(name).png")
        try FileManager.default.createDirectory(
            at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try data.write(to: url)
    }
}

/// `UNUserNotificationCenter` needs a real app bundle, which a test process lacks.
struct FakeNotificationCenter: NotificationCenterClient {
    func requestAuthorization() async throws -> Bool { true }
    func authorizationStatus() async -> UNAuthorizationStatus { .authorized }
    func add(_ notification: MenubarNotification, playSound: Bool) async throws {}
}
