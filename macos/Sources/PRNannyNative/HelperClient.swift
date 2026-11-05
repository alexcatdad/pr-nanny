import Darwin
import Foundation

/// A helper error reply. `message` is fixed, user-safe text the UI may show.
public struct HelperError: Error, LocalizedError, Decodable, Equatable, Sendable {
    public let code: String
    public let message: String
    public var errorDescription: String? { message }

    public init(code: String, message: String) {
        self.code = code
        self.message = message
    }
}

public enum HelperClientError: Error, LocalizedError, Equatable {
    case unavailable, frameTooLarge, invalidSocketPath, invalidResponse

    public var errorDescription: String? {
        switch self {
        case .unavailable: "PR Nanny's background helper is not responding."
        case .frameTooLarge, .invalidSocketPath, .invalidResponse:
            "PR Nanny's background helper returned an invalid reply."
        }
    }
}

public struct EmptyBody: Codable, Equatable, Sendable { public init() {} }

struct RequestEnvelope<Body: Encodable>: Encodable {
    var version = HelperClient.protocolVersion
    let id: String
    let op: String
    let body: Body
}

struct ResponseEnvelope<Result: Decodable>: Decodable {
    let version: Int
    let id: String
    let body: Result?
    let error: HelperError?
}

public struct ResolvedURL: Decodable, Sendable { public let url: String }
public struct HealthReply: Decodable, Sendable { public let status: String }
public struct VersionReply: Decodable, Sendable {
    public let version: String
    public let commit: String
    public let `protocol`: Int
}

/// Version-1 client for the helper's private unix socket (docs/HELPER-API.md):
/// one request per connection, 4-byte big-endian length frames, at most 1 MiB.
public struct HelperClient: Sendable {
    public static let protocolVersion = 1
    public static let maxFrameBytes = 1 << 20

    public let socketPath: String

    public init(socketPath: String) {
        self.socketPath = socketPath
    }

    /// Requests have 5 seconds; account connections wait on GitHub and have 35.
    public static func timeout(for operation: String) -> TimeInterval {
        switch operation {
        case "connectPAT", "connectCLI": 35
        default: 5
        }
    }

    // MARK: Operations

    public func snapshot() throws -> Snapshot { try call("snapshot", body: EmptyBody()) }

    public func saveSettings(_ settings: AppSettings) throws -> Snapshot {
        struct Body: Encodable { let settings: AppSettings }
        return try call("saveSettings", body: Body(settings: settings))
    }

    public func connectPAT(token: String) throws -> Snapshot {
        struct Body: Encodable { let token: String }
        return try call("connectPAT", body: Body(token: token))
    }

    public func connectCLI() throws -> Snapshot { try call("connectCLI", body: EmptyBody()) }

    public func setBell(id: String, enabled: Bool) throws -> Snapshot {
        struct Body: Encodable {
            let id: String
            let enabled: Bool
        }
        return try call("setBell", body: Body(id: id, enabled: enabled))
    }

    public func setLayout(_ layout: String) throws -> Snapshot {
        struct Body: Encodable { let layout: String }
        return try call("setLayout", body: Body(layout: layout))
    }

    /// The only app-initiated GitHub fetch. Progress is read through `snapshot()`.
    public func refresh() throws { let _: EmptyBody = try call("refresh", body: EmptyBody()) }

    public func resolvePR(id: String) throws -> String {
        struct Body: Encodable { let id: String }
        let reply: ResolvedURL = try call("resolvePR", body: Body(id: id))
        return reply.url
    }

    public func nextNotifications(cursor: String, limit: Int = 64) throws -> NotificationBatch {
        struct Body: Encodable {
            let cursor: String
            let limit: Int
        }
        return try call("nextNotifications", body: Body(cursor: cursor, limit: limit))
    }

    public func health() throws -> HealthReply { try call("health", body: EmptyBody()) }
    public func version() throws -> VersionReply { try call("version", body: EmptyBody()) }

    // MARK: Transport

    public func call<Body: Encodable, Result: Decodable>(
        _ operation: String, body: Body, as resultType: Result.Type = Result.self
    ) throws -> Result {
        let requestID = UUID().uuidString.lowercased()
        let request = try JSONEncoder().encode(RequestEnvelope(id: requestID, op: operation, body: body))
        let reply = try exchange(Self.frame(request), timeout: Self.timeout(for: operation))
        return try Self.decodeReply(reply, requestID: requestID)
    }

    static func frame(_ payload: Data) throws -> Data {
        guard payload.count <= maxFrameBytes else { throw HelperClientError.frameTooLarge }
        var length = UInt32(payload.count).bigEndian
        return withUnsafeBytes(of: &length) { Data($0) } + payload
    }

    static func decodeReply<Result: Decodable>(_ data: Data, requestID: String) throws -> Result {
        let response: ResponseEnvelope<Result>
        do {
            response = try GoTime.decoder().decode(ResponseEnvelope<Result>.self, from: data)
        } catch {
            throw HelperClientError.invalidResponse
        }
        guard response.version == protocolVersion, response.id == requestID else {
            throw HelperClientError.invalidResponse
        }
        if let error = response.error { throw error }
        guard let body = response.body else { throw HelperClientError.invalidResponse }
        return body
    }

    private func exchange(_ request: Data, timeout: TimeInterval) throws -> Data {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw HelperClientError.unavailable }
        defer { _ = close(fd) }
        var noSignal: Int32 = 1
        _ = setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &noSignal, socklen_t(MemoryLayout<Int32>.size))
        var limit = timeval(tv_sec: Int(timeout), tv_usec: 0)
        _ = setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &limit, socklen_t(MemoryLayout<timeval>.size))
        _ = setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &limit, socklen_t(MemoryLayout<timeval>.size))

        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(socketPath.utf8) + [0]
        guard pathBytes.count <= MemoryLayout.size(ofValue: address.sun_path) else {
            throw HelperClientError.invalidSocketPath
        }
        withUnsafeMutableBytes(of: &address.sun_path) { $0.copyBytes(from: pathBytes) }
        guard let pathOffset = MemoryLayout<sockaddr_un>.offset(of: \.sun_path) else {
            throw HelperClientError.invalidSocketPath
        }
        let addressLength = socklen_t(pathOffset + pathBytes.count)
        address.sun_len = UInt8(addressLength)
        let connected = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { connect(fd, $0, addressLength) }
        }
        guard connected == 0 else { throw HelperClientError.unavailable }

        try writeAll(fd, request)
        let header = try readExact(fd, count: 4)
        let length = header.reduce(UInt32(0)) { ($0 << 8) | UInt32($1) }
        guard length <= Self.maxFrameBytes else { throw HelperClientError.frameTooLarge }
        return try readExact(fd, count: Int(length))
    }

    private func writeAll(_ fd: Int32, _ bytes: Data) throws {
        try bytes.withUnsafeBytes { raw in
            guard let base = raw.baseAddress else { return }
            var sent = 0
            while sent < raw.count {
                let count = send(fd, base.advanced(by: sent), raw.count - sent, 0)
                guard count > 0 else { throw HelperClientError.unavailable }
                sent += count
            }
        }
    }

    private func readExact(_ fd: Int32, count: Int) throws -> Data {
        var data = Data(count: count)
        try data.withUnsafeMutableBytes { raw in
            guard let base = raw.baseAddress else { return }
            var received = 0
            while received < raw.count {
                let amount = recv(fd, base.advanced(by: received), raw.count - received, 0)
                guard amount > 0 else { throw HelperClientError.unavailable }
                received += amount
            }
        }
        return data
    }
}
