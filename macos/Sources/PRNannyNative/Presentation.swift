import Foundation
import MenubarKit

/// Short friendly ages ("now", "5m", "3h", "2d", "4mo", "1y"); future timestamps read "now".
public func relativeAge(_ date: Date, now: Date = Date()) -> String {
    let seconds = max(0, Int(now.timeIntervalSince(date)))
    switch seconds {
    case ..<60: return "now"
    case ..<3600: return "\(seconds / 60)m"
    case ..<86_400: return "\(seconds / 3600)h"
    case ..<2_592_000: return "\(seconds / 86_400)d"
    case ..<31_536_000: return "\(seconds / 2_592_000)mo"
    default: return "\(seconds / 31_536_000)y"
    }
}

extension CIState {
    public var label: String {
        switch self {
        case .passed: "Passed"
        case .notPassed: "Not passed"
        case .running: "Running"
        case .notAvailable: "Not available"
        }
    }
}

extension PullRequest {
    public var approvalLabel: String { approved ? "Approved" : "Not approved" }

    /// "owner/" and "name" so the row can emphasise the repository name.
    public var repositoryParts: (owner: String, name: String) {
        guard let slash = repository.firstIndex(of: "/"), slash != repository.startIndex else {
            return ("", repository)
        }
        return (String(repository[...slash]), String(repository[repository.index(after: slash)...]))
    }

    public var accessibilityOpenLabel: String { "Open \(repository) pull request #\(number): \(title)" }
    public var bellLabel: String { bell ? "Turn notifications off" : "Turn notifications on" }
}

/// The three lists the panel shows.
public enum PRCategory: String, CaseIterable, Identifiable, Sendable {
    case authored, review, recent

    public var id: String { rawValue }

    public var tabLabel: String {
        switch self {
        case .authored: "Authored"
        case .review: "Review"
        case .recent: "Recent"
        }
    }

    public var sectionTitle: String {
        switch self {
        case .authored: "Authored by you"
        case .review: "Review requested"
        case .recent: "Recently closed"
        }
    }

    public var emptyText: String {
        switch self {
        case .authored: "No open pull requests authored by you."
        case .review: "No review requests right now."
        case .recent: "No recently closed pull requests."
        }
    }

    /// Pull requests closed less than this long ago are "recent".
    public static let recentWindow: TimeInterval = 60

    public func includes(_ pr: PullRequest, now: Date) -> Bool {
        switch self {
        case .authored: return pr.authored && pr.closedAt == nil
        case .review: return pr.reviewRequested && pr.closedAt == nil
        case .recent:
            guard let closed = pr.closedAt else { return false }
            return now >= closed && now.timeIntervalSince(closed) < Self.recentWindow
        }
    }

    /// Newest update first.
    public func filter(_ prs: [PullRequest], now: Date) -> [PullRequest] {
        PRCategory.sorted(prs).filter { includes($0, now: now) }
    }

    public static func sorted(_ prs: [PullRequest]) -> [PullRequest] {
        prs.sorted { $0.updatedAt > $1.updatedAt }
    }
}

public enum PanelLayout: String, CaseIterable, Sendable {
    case all, tabs

    public init(settings: AppSettings) { self = PanelLayout(rawValue: settings.layout) ?? .all }

    public var label: String {
        switch self {
        case .all: "Show all"
        case .tabs: "Tabs"
        }
    }
}

/// The helper process as the app sees it.
public enum HelperState: Equatable, Sendable {
    case starting
    case running
    /// Another PR Nanny already runs for this user.
    case alreadyRunning
    case unavailable(String)
}

/// Connection words shared by the tray, panel and Settings.
public enum ConnectionPresentation {
    public static let authRequiredStates: Set<String> = [
        "disconnected", "missing_auth", "auth_error", "authentication_error",
    ]

    public static func authRequired(_ connection: String) -> Bool { authRequiredStates.contains(connection) }

    /// Auth problems after a working connection read "Reconnect", first runs "Connect".
    public static func needsReconnect(_ connection: String) -> Bool {
        ["error", "auth_error", "authentication_error", "rate_limited"].contains(connection)
    }

    public static func connectionLine(_ snapshot: Snapshot) -> String {
        switch snapshot.connection {
        case "connected":
            snapshot.settings.account.isEmpty ? "Connected" : "Connected as \(snapshot.settings.account)"
        case "offline": "Offline · showing saved pull requests"
        case "rate_limited": "GitHub rate limit reached"
        case "error": "Connection problem"
        case "auth_error", "authentication_error": "Authentication expired"
        default: "Not connected"
        }
    }

    public static func lastRefresh(_ snapshot: Snapshot, now: Date = Date()) -> String {
        guard let last = snapshot.lastRefresh else { return "" }
        let age = relativeAge(last, now: now)
        return age == "now" ? "Updated just now" : "Updated \(age) ago"
    }

    /// The info banner: the helper's own message, else a fixed sentence for the connection.
    public static func statusMessage(_ snapshot: Snapshot) -> String {
        if !snapshot.message.isEmpty { return snapshot.message }
        switch snapshot.connection {
        case "offline": return "You are offline. Showing the pull requests saved from your last refresh."
        case "rate_limited": return "GitHub is limiting requests. PR Nanny will try again automatically."
        case "error": return "GitHub could not be reached. PR Nanny will retry automatically."
        default: return ""
        }
    }

    /// Whether the info banner shows (an action error banner takes precedence in the view).
    public static func showsBanner(_ snapshot: Snapshot) -> Bool {
        let message = statusMessage(snapshot)
        return !message.isEmpty && (snapshot.connection != "connected" || !snapshot.message.isEmpty)
    }

    /// One status for the tray label and the panel header.
    public static func status(helper: HelperState, snapshot: Snapshot?, now: Date = Date()) -> MenubarStatus {
        switch helper {
        case .alreadyRunning:
            return MenubarStatus(
                tone: .error, word: "Already running", headline: "PR Nanny is already running",
                detail: "Quit the other copy of PR Nanny, then try again.")
        case .unavailable(let message):
            return MenubarStatus(
                tone: .error, word: "Unavailable", headline: "PR Nanny is unavailable", detail: message)
        case .starting, .running:
            break
        }
        guard let snapshot else {
            return MenubarStatus(tone: .off, word: "Starting", headline: "PR Nanny", detail: "Loading pull requests…")
        }
        let line = connectionLine(snapshot)
        let updated = lastRefresh(snapshot, now: now)
        let detail = updated.isEmpty ? line : "\(line) · \(updated)"
        if authRequired(snapshot.connection) {
            return MenubarStatus(tone: .warning, word: "Sign-in needed", headline: "PR Nanny", detail: line)
        }
        switch snapshot.connection {
        case "connected":
            let open = snapshot.pullRequests.filter { $0.closedAt == nil }.count
            let word = open == 1 ? "1 open pull request" : "\(open) open pull requests"
            return MenubarStatus(tone: .ok, word: word, headline: "PR Nanny", detail: detail)
        case "offline", "rate_limited":
            return MenubarStatus(tone: .warning, word: line, headline: "PR Nanny", detail: detail)
        default:
            return MenubarStatus(tone: .error, word: line, headline: "PR Nanny", detail: detail)
        }
    }

    public static func countLabel(_ count: Int) -> String {
        count == 1 ? "1 pull request" : "\(count) pull requests"
    }
}
