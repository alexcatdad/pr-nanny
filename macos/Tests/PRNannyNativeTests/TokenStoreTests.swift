import Darwin
import Foundation
import Security
import Testing

@testable import PRNannyNative

/// An in-memory stand-in for both keychains. Without the entitlement the
/// data-protection keychain answers `errSecMissingEntitlement`, like an
/// ad-hoc build.
final class FakeKeychain: @unchecked Sendable {
    struct Item: Equatable {
        let dataProtection: Bool
        let account: String
        let token: String
        let accessible: String?
        let accessGroup: String?
    }

    private let lock = NSLock()
    private var stored: [Item] = []
    var hasEntitlement: Bool
    var failStatus: OSStatus?

    init(hasEntitlement: Bool) { self.hasEntitlement = hasEntitlement }

    var items: [Item] { lock.withLock { stored } }

    func seed(_ item: Item) { lock.withLock { stored.append(item) } }

    var calls: KeychainCalls {
        KeychainCalls(
            add: { attributes in self.add(attributes) },
            copyData: { query in self.copy(query) },
            delete: { query in self.delete(query) })
    }

    private func isDataProtection(_ query: [String: Any]) -> Bool {
        query[kSecUseDataProtectionKeychain as String] as? Bool == true
    }

    private func gate(_ query: [String: Any]) -> OSStatus? {
        if let failStatus { return failStatus }
        if isDataProtection(query) && !hasEntitlement { return errSecMissingEntitlement }
        return nil
    }

    private func matches(_ item: Item, _ query: [String: Any]) -> Bool {
        item.dataProtection == isDataProtection(query)
            && (query[kSecAttrAccount as String] as? String).map { $0 == item.account } ?? true
    }

    private func add(_ attributes: [String: Any]) -> OSStatus {
        lock.withLock {
            if let status = gate(attributes) { return status }
            let account = attributes[kSecAttrAccount as String] as? String ?? ""
            if stored.contains(where: { matches($0, attributes) && $0.account == account }) {
                return errSecDuplicateItem
            }
            let data = attributes[kSecValueData as String] as? Data ?? Data()
            stored.append(
                Item(
                    dataProtection: isDataProtection(attributes), account: account,
                    token: String(decoding: data, as: UTF8.self),
                    accessible: attributes[kSecAttrAccessible as String] as? String,
                    accessGroup: attributes[kSecAttrAccessGroup as String] as? String))
            return errSecSuccess
        }
    }

    private func copy(_ query: [String: Any]) -> (OSStatus, Data?) {
        lock.withLock {
            if let status = gate(query) { return (status, nil) }
            guard let item = stored.first(where: { matches($0, query) }) else { return (errSecItemNotFound, nil) }
            return (errSecSuccess, Data(item.token.utf8))
        }
    }

    private func delete(_ query: [String: Any]) -> OSStatus {
        lock.withLock {
            if let status = gate(query) { return status }
            let before = stored.count
            stored.removeAll { matches($0, query) }
            return stored.count == before ? errSecItemNotFound : errSecSuccess
        }
    }
}

private let bundleID = "com.alexcatdad.pr-nanny"

@Test func savesToTheDataProtectionKeychainWithTheTeamGroup() throws {
    let keychain = FakeKeychain(hasEntitlement: true)
    let store = KeychainTokenStore(bundleID: bundleID, calls: keychain.calls)
    #expect(store.service == "com.alexcatdad.pr-nanny.github-token")
    #expect(KeychainTokenStore.teamID == "CX6D6KGCT5")
    let group = "\(KeychainTokenStore.teamID).\(bundleID)"
    #expect(store.accessGroup == group)

    try store.save("synthetic-one", for: "octocat")
    try store.save("synthetic-two", for: "hubot")
    #expect(
        keychain.items == [
            .init(
                dataProtection: true, account: "hubot", token: "synthetic-two",
                accessible: kSecAttrAccessibleWhenUnlockedThisDeviceOnly as String,
                accessGroup: group)
        ])
    #expect(try store.token(for: "hubot") == "synthetic-two")
    #expect(try store.token(for: "octocat") == nil)

    try store.deleteAll()
    #expect(keychain.items.isEmpty)
    try store.deleteAll()
}

@Test func fallsBackToTheLoginKeychainWithoutTheEntitlement() throws {
    let keychain = FakeKeychain(hasEntitlement: false)
    let store = KeychainTokenStore(bundleID: "\(bundleID).dev", calls: keychain.calls)
    try store.save("synthetic", for: "octocat")
    #expect(keychain.items.map(\.dataProtection) == [false])
    #expect(keychain.items.first?.accessible == nil)
    #expect(try store.token(for: "octocat") == "synthetic")
    #expect(try store.token(for: "hubot") == nil)
    try store.deleteAll()
    #expect(keychain.items.isEmpty)
}

@Test func movesALoginKeychainTokenOnceTheEntitlementArrives() throws {
    let keychain = FakeKeychain(hasEntitlement: true)
    keychain.seed(
        .init(dataProtection: false, account: "octocat", token: "synthetic", accessible: nil, accessGroup: nil))
    let store = KeychainTokenStore(bundleID: bundleID, calls: keychain.calls)
    #expect(try store.token(for: "octocat") == "synthetic")
    #expect(keychain.items.map(\.dataProtection) == [true])
    #expect(try store.token(for: "hubot") == nil)
}

@Test func reportsKeychainFailures() {
    let keychain = FakeKeychain(hasEntitlement: true)
    keychain.failStatus = errSecInteractionNotAllowed
    let store = KeychainTokenStore(bundleID: bundleID, calls: keychain.calls)
    #expect(throws: TokenStoreError(status: errSecInteractionNotAllowed)) { try store.token(for: "octocat") }
    #expect(throws: TokenStoreError(status: errSecInteractionNotAllowed)) { try store.save("x", for: "octocat") }
    #expect(throws: TokenStoreError(status: errSecInteractionNotAllowed)) { try store.deleteAll() }
}

@Test func rejectsUndecodableItems() {
    let calls = KeychainCalls(
        add: { _ in errSecSuccess }, copyData: { _ in (errSecSuccess, Data([0xff, 0xfe])) },
        delete: { _ in errSecSuccess })
    #expect(throws: TokenStoreError(status: errSecDecode)) {
        try KeychainTokenStore(bundleID: bundleID, calls: calls).token(for: "octocat")
    }
}

@Test func failsWhenNeitherKeychainAcceptsTheItem() {
    let calls = KeychainCalls(
        add: { _ in errSecMissingEntitlement }, copyData: { _ in (errSecItemNotFound, nil) },
        delete: { _ in errSecItemNotFound })
    #expect(throws: TokenStoreError(status: errSecMissingEntitlement)) {
        try KeychainTokenStore(bundleID: bundleID, calls: calls).save("x", for: "octocat")
    }
}

// MARK: Handoff

final class MemoryTokenStore: TokenStore, @unchecked Sendable {
    private let lock = NSLock()
    private var saved: (account: String, token: String)?
    var failRead = false
    var failSave = false

    init(_ saved: (String, String)? = nil) { self.saved = saved.map { (account: $0.0, token: $0.1) } }

    var current: (account: String, token: String)? { lock.withLock { saved } }

    func token(for account: String) throws -> String? {
        if failRead { throw TokenStoreError(status: errSecInteractionNotAllowed) }
        return lock.withLock { saved?.account == account ? saved?.token : nil }
    }
    func save(_ token: String, for account: String) throws {
        if failSave { throw TokenStoreError(status: errSecInteractionNotAllowed) }
        lock.withLock { saved = (account, token) }
    }
    func deleteAll() throws { lock.withLock { saved = nil } }
}

final class FakeTokenHelper: HelperTokenAPI, @unchecked Sendable {
    private let lock = NSLock()
    private var log: [String] = []
    var legacy: LegacyToken?
    var failProvide = false
    var failLegacy = false
    var failDelete = false

    var calls: [String] { lock.withLock { log } }
    private func record(_ call: String) { lock.withLock { log.append(call) } }

    func provideToken(account: String, token: String) throws -> Snapshot {
        record("provide:\(account):\(token)")
        if failProvide { throw HelperError(code: "AUTHENTICATION", message: "x") }
        return fixtureSnapshot()
    }
    func legacyToken() throws -> LegacyToken? {
        record("legacy")
        if failLegacy { throw HelperError(code: "CREDENTIAL_STORE", message: "x") }
        return legacy
    }
    func deleteLegacyToken() throws {
        record("deleteLegacy")
        if failDelete { throw HelperError(code: "CREDENTIAL_STORE", message: "x") }
    }
}

private let pat = AppSettings(authMethod: "pat", account: "octocat")

@Test func handsTheSavedTokenToTheHelper() {
    let store = MemoryTokenStore(("octocat", "synthetic"))
    let helper = FakeTokenHelper()
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .provided)
    #expect(helper.calls == ["provide:octocat:synthetic"])

    helper.failProvide = true
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .failed(TokenHandoff.readFailed))
    store.failRead = true
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .failed(TokenHandoff.readFailed))
}

@Test func migratesTheLegacyItemOnceThenDeletesIt() {
    let store = MemoryTokenStore()
    let helper = FakeTokenHelper()
    helper.legacy = LegacyToken(account: "octocat", token: "synthetic-legacy")
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .migrated)
    #expect(helper.calls == ["legacy", "deleteLegacy"])
    #expect(store.current?.token == "synthetic-legacy")
    // The next launch finds the new item and never asks for the legacy one.
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .provided)
    #expect(helper.calls.last == "provide:octocat:synthetic-legacy")
}

@Test func keepsTheLegacyItemWhenMigrationFails() {
    let helper = FakeTokenHelper()
    helper.legacy = LegacyToken(account: "octocat", token: "synthetic-legacy")
    let store = MemoryTokenStore()
    store.failSave = true
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .failed(TokenHandoff.migrationFailed))
    #expect(helper.calls == ["legacy"])

    store.failSave = false
    helper.failDelete = true
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .failed(TokenHandoff.migrationFailed))
    // The token is saved anyway, so the next launch uses it.
    #expect(TokenHandoff.start(settings: pat, store: store, helper: helper) == .provided)

    let fresh = MemoryTokenStore()
    #expect(TokenHandoff.start(settings: pat, store: fresh, helper: helper) == .failed(TokenHandoff.migrationFailed))
    helper.failLegacy = true
    #expect(
        TokenHandoff.start(settings: pat, store: MemoryTokenStore(), helper: helper)
            == .failed(TokenHandoff.migrationFailed))
}

@Test func reportsAMissingToken() {
    #expect(TokenHandoff.start(settings: pat, store: MemoryTokenStore(), helper: FakeTokenHelper()) == .missing)
}

@Test func removesTheTokenWhenThePATIsNoLongerUsed() {
    let store = MemoryTokenStore(("octocat", "synthetic"))
    let helper = FakeTokenHelper()
    #expect(
        TokenHandoff.start(settings: AppSettings(authMethod: "gh", account: "octocat"), store: store, helper: helper)
            == .notNeeded)
    #expect(store.current == nil)
    #expect(helper.calls.isEmpty)

    let other = MemoryTokenStore(("octocat", "synthetic"))
    TokenHandoff.forgetUnused(settings: pat, store: other)
    #expect(other.current != nil)
    TokenHandoff.forgetUnused(settings: AppSettings(authMethod: "gh"), store: other)
    #expect(other.current == nil)
}

@Test func savesAConnectedToken() {
    let store = MemoryTokenStore()
    #expect(TokenHandoff.saveConnected(token: "synthetic", settings: pat, store: store) == nil)
    #expect(store.current?.account == "octocat")
    #expect(TokenHandoff.saveConnected(token: "synthetic", settings: AppSettings(), store: store) == nil)
    store.failSave = true
    #expect(TokenHandoff.saveConnected(token: "synthetic", settings: pat, store: store) == TokenHandoff.saveFailed)
}

// MARK: Wire

/// Serves one canned reply per connection on a unix socket and records each request.
final class OneShotHelper: @unchecked Sendable {
    let path: String
    private let fd: Int32
    private let lock = NSLock()
    private var seen: [[String: Any]] = []

    var requests: [[String: Any]] { lock.withLock { seen } }

    init(replies: [(body: String?, error: String?)]) throws {
        path = "/tmp/pn-\(UUID().uuidString.prefix(8)).sock"
        let listener = socket(AF_UNIX, SOCK_STREAM, 0)
        fd = listener
        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        let bytes = Array(path.utf8) + [0]
        withUnsafeMutableBytes(of: &address.sun_path) { $0.copyBytes(from: bytes) }
        let size = socklen_t(MemoryLayout<sockaddr_un>.size)
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(listener, $0, size) }
        }
        guard bound == 0, listen(listener, 4) == 0 else { throw HelperClientError.unavailable }
        Thread.detachNewThread { [self] in
            for reply in replies {
                let conn = accept(listener, nil, nil)
                guard conn >= 0 else { return }
                var header = [UInt8](repeating: 0, count: 4)
                _ = recv(conn, &header, 4, MSG_WAITALL)
                let length = header.reduce(0) { ($0 << 8) | Int($1) }
                var payload = [UInt8](repeating: 0, count: length)
                _ = recv(conn, &payload, length, MSG_WAITALL)
                let request = (try? JSONSerialization.jsonObject(with: Data(payload))) as? [String: Any] ?? [:]
                lock.withLock { seen.append(request) }
                let id = request["id"] as? String ?? ""
                let field = reply.error.map { "\"error\":\($0)" } ?? "\"body\":\(reply.body ?? "{}")"
                let response = Array("{\"version\":1,\"id\":\"\(id)\",\(field)}".utf8)
                let frame = withUnsafeBytes(of: UInt32(response.count).bigEndian) { Array($0) } + response
                _ = send(conn, frame, frame.count, 0)
                close(conn)
            }
        }
    }

    deinit {
        close(fd)
        unlink(path)
    }
}

@Test func tokenOperationsUseTheWireNames() throws {
    let server = try OneShotHelper(replies: [
        (#"{"account":"octocat","token":"synthetic-legacy"}"#, nil),
        (nil, #"{"code":"NOT_FOUND","message":"none"}"#),
        ("{}", nil),
        (nil, #"{"code":"AUTHENTICATION","message":"no"}"#),
    ])
    let client = HelperClient(socketPath: server.path)
    #expect(try client.legacyToken() == LegacyToken(account: "octocat", token: "synthetic-legacy"))
    #expect(try client.legacyToken() == nil)
    try client.deleteLegacyToken()
    #expect(throws: HelperError(code: "AUTHENTICATION", message: "no")) {
        try client.provideToken(account: "octocat", token: "synthetic")
    }
    let requests = server.requests
    #expect(
        requests.map { $0["op"] as? String } == ["legacyToken", "legacyToken", "deleteLegacyToken", "provideToken"])
    let body = requests.last?["body"] as? [String: String]
    #expect(body == ["account": "octocat", "token": "synthetic"])
}
