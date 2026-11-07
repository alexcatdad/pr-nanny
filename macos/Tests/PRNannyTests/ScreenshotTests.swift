import AppKit
import Foundation
import MenubarKit
import SwiftUI
import Testing

@testable import PRNanny
@testable import PRNannyNative

/// Website and README screenshots, drawn offscreen from made-up fixtures by
/// `PanelSnapshot`. Runs only when `PR_NANNY_SCREENSHOTS=<dir>` is set
/// (`task screenshots`); no GitHub account data is involved.
@MainActor
@Suite(.enabled(if: screenshotDirectory() != nil))
struct ScreenshotTests {

    static let appearances: [(suffix: String, name: NSAppearance.Name)] = [("light", .aqua), ("dark", .darkAqua)]

    /// Authored and review-requested PRs in fictional repositories, with mixed checks.
    static func pullRequests(now: Date) -> [PullRequest] {
        func pr(
            _ number: Int, _ repository: String, _ title: String, age hours: Double, updated minutes: Double,
            approved: Bool = false, ci: CIState, authored: Bool = false, review: Bool = false, bell: Bool = false,
            mergedSecondsAgo: Double? = nil
        ) -> PullRequest {
            PullRequest(
                id: "PR_fixture_\(repository)_\(number)", number: number, title: title, repository: repository,
                url: "https://github.com/\(repository)/pull/\(number)",
                createdAt: now.addingTimeInterval(-hours * 3600), updatedAt: now.addingTimeInterval(-minutes * 60),
                closedAt: mergedSecondsAgo.map { now.addingTimeInterval(-$0) }, merged: mergedSecondsAgo != nil,
                approved: approved, ci: ci, authored: authored, reviewRequested: review, bell: bell)
        }
        return [
            pr(
                482, "acme/web", "Move checkout to the new address form", age: 26, updated: 4, approved: true,
                ci: .passed, authored: true, bell: true),
            pr(
                1197, "acme/api", "Rate-limit the export endpoint per workspace", age: 5, updated: 12, ci: .running,
                authored: true),
            pr(
                76, "acme/design-tokens", "Darken the muted text colour for contrast", age: 50, updated: 95,
                ci: .notPassed, authored: true, bell: true),
            pr(
                1203, "acme/api", "Add cursor pagination to /v2/invoices", age: 3, updated: 8, ci: .passed,
                review: true),
            pr(
                489, "acme/web", "Lazy-load the onboarding illustrations", age: 20, updated: 41, approved: true,
                ci: .notAvailable, review: true),
            pr(
                31, "acme/infra", "Pin the runner image by digest", age: 74, updated: 1, approved: true, ci: .passed,
                authored: true, mergedSecondsAgo: 20),
        ]
    }

    /// A bundle whose Info.plist carries `$PR_NANNY_SCREENSHOT_VERSION` (the Taskfile passes the
    /// latest tag), so the version shown is not "unknown".
    static func versionBundle() throws -> Bundle {
        let tag = ProcessInfo.processInfo.environment["PR_NANNY_SCREENSHOT_VERSION"] ?? ""
        let root = FileManager.default.temporaryDirectory.appending(
            path: "pr-nanny-screenshots-\(UUID().uuidString).bundle")
        let contents = root.appending(path: "Contents")
        try FileManager.default.createDirectory(at: contents, withIntermediateDirectories: true)
        let info: [String: String] = [
            "CFBundleIdentifier": "com.alexcatdad.pr-nanny.screenshots",
            "ReleaseVersion": tag.hasPrefix("v") ? String(tag.dropFirst()) : tag,
        ]
        try PropertyListSerialization.data(fromPropertyList: info, format: .xml, options: 0)
            .write(to: contents.appending(path: "Info.plist"))
        return try #require(Bundle(url: root))
    }

    static func model(layout: PanelLayout) -> AppModel {
        let now = Date()
        let model = AppModel(notifier: MenubarNotifier(center: FakeNotificationCenter()))
        var settings = AppSettings(account: "octo-fixture")
        settings.layout = layout.rawValue
        model.showForTesting(
            Snapshot(
                settings: settings, pullRequests: pullRequests(now: now), connection: "connected", message: "",
                lastRefresh: now.addingTimeInterval(-40)))
        return model
    }

    static func write(_ view: some View, name: String, width: CGFloat, chrome: PanelSnapshot.Chrome) throws {
        let directory = try #require(screenshotDirectory())
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        for appearance in appearances {
            let data = try #require(
                PanelSnapshot.png(of: view, width: width, chrome: chrome, appearance: appearance.name),
                "\(name)-\(appearance.suffix)")
            try data.write(to: directory.appending(path: "\(name)-\(appearance.suffix).png"))
        }
    }

    @Test func panel() throws {
        var view = PanelView()
        view.versionBundle = try Self.versionBundle()
        try Self.write(
            view.environmentObject(Self.model(layout: .all)), name: "panel",
            width: MenubarMetrics.panelWidth,
            chrome: .panel)
    }

    @Test func panelTabs() throws {
        var view = PanelView()
        view.versionBundle = try Self.versionBundle()
        try Self.write(
            view.environmentObject(Self.model(layout: .tabs)),
            name: "panel-tabs",
            width: MenubarMetrics.panelWidth, chrome: .panel)
    }

    @Test func settings() throws {
        var view = SettingsView()
        view.versionBundle = try Self.versionBundle()
        try Self.write(
            view.environmentObject(Self.model(layout: .all)), name: "settings",
            width: MenubarMetrics.settingsWidth, chrome: .window(title: "PR Nanny Settings"))
    }
}

/// `$PR_NANNY_SCREENSHOTS`, or nil when screenshots are not requested.
func screenshotDirectory() -> URL? {
    guard let path = ProcessInfo.processInfo.environment["PR_NANNY_SCREENSHOTS"], !path.isEmpty else { return nil }
    return URL(filePath: path, directoryHint: .isDirectory)
}
