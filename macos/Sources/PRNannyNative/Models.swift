import Foundation

/// CI summary of a pull request. Unknown values from a newer helper decode as `.notAvailable`.
public enum CIState: String, Sendable, Equatable {
    case passed
    case notPassed = "not_passed"
    case running
    case notAvailable = "not_available"
}

extension CIState: Codable {
    public init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = CIState(rawValue: raw) ?? .notAvailable
    }
}

/// `model.PullRequest` from the helper. All text is untrusted API data: render it as plain text only.
public struct PullRequest: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var number: Int
    public var title: String
    public var repository: String
    public var url: String
    public var createdAt: Date
    public var updatedAt: Date
    public var closedAt: Date?
    public var merged: Bool
    public var approved: Bool
    public var ci: CIState
    public var authored: Bool
    public var reviewRequested: Bool
    public var bell: Bool

    public init(
        id: String, number: Int, title: String, repository: String, url: String, createdAt: Date,
        updatedAt: Date, closedAt: Date? = nil, merged: Bool = false, approved: Bool = false,
        ci: CIState = .notAvailable, authored: Bool = false, reviewRequested: Bool = false, bell: Bool = false
    ) {
        self.id = id
        self.number = number
        self.title = title
        self.repository = repository
        self.url = url
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.closedAt = closedAt
        self.merged = merged
        self.approved = approved
        self.ci = ci
        self.authored = authored
        self.reviewRequested = reviewRequested
        self.bell = bell
    }
}

/// `model.EventPreferences`: which monitor events may notify.
public struct EventPreferences: Codable, Equatable, Sendable {
    public var ci: Bool
    public var approval: Bool
    public var reviewRequested: Bool
    public var merged: Bool
    public var closed: Bool

    public init(
        ci: Bool = true, approval: Bool = true, reviewRequested: Bool = true, merged: Bool = true, closed: Bool = true
    ) {
        self.ci = ci
        self.approval = approval
        self.reviewRequested = reviewRequested
        self.merged = merged
        self.closed = closed
    }
}

/// `model.Settings`. The helper rejects unknown fields, so this mirrors the Go struct exactly.
public struct AppSettings: Codable, Equatable, Sendable {
    public var version: Int
    public var authMethod: String
    public var account: String
    public var intervalSeconds: Int
    public var layout: String
    public var notifications: Bool
    public var events: EventPreferences
    public var launchAtLogin: Bool

    public init(
        version: Int = 1, authMethod: String = "gh", account: String = "", intervalSeconds: Int = 60,
        layout: String = "all", notifications: Bool = true, events: EventPreferences = EventPreferences(),
        launchAtLogin: Bool = true
    ) {
        self.version = version
        self.authMethod = authMethod
        self.account = account
        self.intervalSeconds = intervalSeconds
        self.layout = layout
        self.notifications = notifications
        self.events = events
        self.launchAtLogin = launchAtLogin
    }
}

/// `model.Snapshot`: the helper's cached state. It never contains tokens.
public struct Snapshot: Codable, Equatable, Sendable {
    public var settings: AppSettings
    public var pullRequests: [PullRequest]
    public var connection: String
    public var message: String
    public var lastRefresh: Date?
    public var refreshing: Bool
    public var degraded: Bool

    public init(
        settings: AppSettings = AppSettings(), pullRequests: [PullRequest] = [], connection: String = "disconnected",
        message: String = "", lastRefresh: Date? = nil, refreshing: Bool = false, degraded: Bool = false
    ) {
        self.settings = settings
        self.pullRequests = pullRequests
        self.connection = connection
        self.message = message
        self.lastRefresh = lastRefresh
        self.refreshing = refreshing
        self.degraded = degraded
    }

    private enum CodingKeys: String, CodingKey {
        case settings, pullRequests, connection, message, lastRefresh, refreshing, degraded
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        settings = try values.decode(AppSettings.self, forKey: .settings)
        // Go encodes an empty slice as null.
        pullRequests = try values.decodeIfPresent([PullRequest].self, forKey: .pullRequests) ?? []
        connection = try values.decode(String.self, forKey: .connection)
        message = try values.decodeIfPresent(String.self, forKey: .message) ?? ""
        lastRefresh = try values.decodeIfPresent(Date.self, forKey: .lastRefresh)
        refreshing = try values.decodeIfPresent(Bool.self, forKey: .refreshing) ?? false
        degraded = try values.decodeIfPresent(Bool.self, forKey: .degraded) ?? false
    }
}

/// One gated monitor event from `nextNotifications`.
public struct HelperNotification: Codable, Equatable, Sendable {
    /// sha256-derived dedupe ID; reused as the notification identifier so repeats coalesce.
    public var id: String
    public var prId: String
    public var event: String
    public var title: String
    public var subtitle: String
    public var body: String

    public init(id: String, prId: String, event: String, title: String, subtitle: String, body: String) {
        self.id = id
        self.prId = prId
        self.event = event
        self.title = title
        self.subtitle = subtitle
        self.body = body
    }
}

public struct NotificationBatch: Codable, Equatable, Sendable {
    public var notifications: [HelperNotification]
    public var cursor: String
    public var missed: Bool

    private enum CodingKeys: String, CodingKey { case notifications, cursor, missed }

    public init(notifications: [HelperNotification], cursor: String, missed: Bool = false) {
        self.notifications = notifications
        self.cursor = cursor
        self.missed = missed
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        notifications = try values.decodeIfPresent([HelperNotification].self, forKey: .notifications) ?? []
        cursor = try values.decode(String.self, forKey: .cursor)
        missed = try values.decodeIfPresent(Bool.self, forKey: .missed) ?? false
    }
}

/// Decodes Go `time.Time` JSON (RFC 3339 with up to nanosecond fractions).
public enum GoTime {
    public static func parse(_ value: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        guard let dot = value.firstIndex(of: ".") else { return formatter.date(from: value) }
        let afterDot = value[value.index(after: dot)...]
        guard let zone = afterDot.firstIndex(where: { !$0.isNumber }) else { return nil }
        let digits = afterDot[..<zone]
        guard !digits.isEmpty, let base = formatter.date(from: String(value[..<dot]) + value[zone...]) else {
            return nil
        }
        let fraction = Double("0." + digits) ?? 0
        return base.addingTimeInterval(fraction)
    }

    public static func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let raw = try container.decode(String.self)
            guard let date = parse(raw) else {
                throw DecodingError.dataCorruptedError(in: container, debugDescription: "Invalid timestamp")
            }
            return date
        }
        return decoder
    }
}
