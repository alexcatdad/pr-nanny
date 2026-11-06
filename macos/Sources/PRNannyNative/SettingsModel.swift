import Foundation
import MenubarKit
import UserNotifications

/// Refresh intervals the helper accepts (storage.validateSettings).
public struct RefreshInterval: Identifiable, Equatable, Sendable {
    public let seconds: Int
    public let label: String
    public var id: Int { seconds }

    public static let all = [
        RefreshInterval(seconds: 30, label: "30 seconds"),
        RefreshInterval(seconds: 60, label: "60 seconds"),
        RefreshInterval(seconds: 120, label: "2 minutes"),
        RefreshInterval(seconds: 300, label: "5 minutes"),
    ]
}

public enum AuthMethod: String, CaseIterable, Identifiable, Sendable {
    case gh, pat
    public var id: String { rawValue }
    public var label: String {
        switch self {
        case .gh: "GitHub CLI"
        case .pat: "Personal access token"
        }
    }
}

/// One per-event notification preference.
public struct EventToggle: Identifiable, Sendable {
    public let id: String
    public let label: String
    public let keyPath: WritableKeyPath<EventPreferences, Bool> & Sendable

    public static let all = [
        EventToggle(id: "ci", label: "CI status changes", keyPath: \.ci),
        EventToggle(id: "approval", label: "Approval changes", keyPath: \.approval),
        EventToggle(id: "reviewRequested", label: "Review requests", keyPath: \.reviewRequested),
        EventToggle(id: "merged", label: "Pull request merged", keyPath: \.merged),
        EventToggle(id: "closed", label: "Pull request closed", keyPath: \.closed),
    ]
}

/// Fixed user-facing sentences for Settings actions; helper text is never shown raw.
public enum SettingsNotice {
    public static let saved = ""
    public static let saveFailed = "Could not save settings. Try again."
    public static let patConnected = "GitHub connected."
    public static let patFailed = "Could not validate that token. Check it and try again."
    public static let cliConnected = "GitHub CLI connected."
    public static let cliFailed = "Could not find an authenticated GitHub CLI session."
    public static let loginItemNeedsApproval = "Allow PR Nanny in System Settings › General › Login Items."
    public static let loginItemFailed = "Could not change the login item. Try again."
    public static let hint =
        "New review requests notify automatically. Other changes notify only for pull requests with their bell enabled."
}

public enum SettingsRules {
    /// The helper cannot change the authentication method or account through
    /// `saveSettings`, so a save always carries the current ones.
    public static func preferencesToSave(_ edited: AppSettings, current: AppSettings) -> AppSettings {
        var settings = edited
        settings.authMethod = current.authMethod
        settings.account = current.account
        return settings
    }

    /// Settings with notifications switched as the user's master toggle says.
    public static func withNotifications(_ enabled: Bool, current: AppSettings) -> AppSettings {
        var settings = current
        settings.notifications = enabled
        return settings
    }
}

/// Maps a helper notification to what MenubarKit posts. The dedupe ID is the
/// identifier, the PR node ID travels in `userInfo` (so a click can resolve the
/// PR even after a relaunch) and groups the PR's notifications in one thread.
public enum NotificationMapping {
    public static let prIDKey = "prId"

    public static func notification(for entry: HelperNotification) -> MenubarNotification {
        MenubarNotification(
            title: entry.title, subtitle: entry.subtitle.isEmpty ? nil : entry.subtitle, body: entry.body,
            threadIdentifier: entry.prId, identifier: entry.id, userInfo: [prIDKey: entry.prId])
    }

    /// The PR to open for a click, or nil for a dismissal or a foreign notification.
    public static func prID(for click: NotificationClick) -> String? {
        guard !click.isDismissal, click.actionIdentifier == UNNotificationDefaultActionIdentifier else { return nil }
        let id = click.userInfo[prIDKey] ?? click.threadIdentifier ?? ""
        return id.isEmpty ? nil : id
    }
}
