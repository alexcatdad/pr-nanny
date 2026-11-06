import Foundation
import Security

/// Where the app keeps the GitHub personal access token. The helper only ever
/// holds it in memory, handed over the private socket by `TokenHandoff`.
public protocol TokenStore: Sendable {
    /// The saved token for `account`, or nil when there is none.
    func token(for account: String) throws -> String?
    /// Saves `token` as the only token, replacing any other account's.
    func save(_ token: String, for account: String) throws
    /// Removes every saved token.
    func deleteAll() throws
}

public struct TokenStoreError: Error, Equatable, Sendable {
    public let status: OSStatus
    public init(status: OSStatus) { self.status = status }
}

/// The four SecItem calls `KeychainTokenStore` makes, so tests can run
/// without touching a real keychain.
public struct KeychainCalls: Sendable {
    public var add: @Sendable ([String: Any]) -> OSStatus
    public var copyData: @Sendable ([String: Any]) -> (OSStatus, Data?)
    public var delete: @Sendable ([String: Any]) -> OSStatus

    public init(
        add: @escaping @Sendable ([String: Any]) -> OSStatus,
        copyData: @escaping @Sendable ([String: Any]) -> (OSStatus, Data?),
        delete: @escaping @Sendable ([String: Any]) -> OSStatus
    ) {
        self.add = add
        self.copyData = copyData
        self.delete = delete
    }

    public static let system = KeychainCalls(
        add: { SecItemAdd($0 as CFDictionary, nil) },
        copyData: { query in
            var result: CFTypeRef?
            let status = SecItemCopyMatching(query as CFDictionary, &result)
            return (status, result as? Data)
        },
        delete: { SecItemDelete($0 as CFDictionary) })
}

/// A generic-password item in the data-protection keychain, readable only
/// while the Mac is unlocked, never synced or migrated to another device, and
/// shared only within the team access group.
///
/// The data-protection keychain needs the `keychain-access-groups` entitlement,
/// which a Developer ID app gets only with an embedded provisioning profile
/// (see build/SIGNING.md). Without it (ad-hoc dev builds, or a release signed
/// without the profile) SecItem reports `errSecMissingEntitlement`, and the
/// item goes to the login keychain instead, where its ACL trusts only this
/// app. Either way `security find-generic-password` cannot read it silently.
public struct KeychainTokenStore: TokenStore {
    public static let teamID = "CX6D6KGCT5"

    public let service: String
    public let accessGroup: String
    private let calls: KeychainCalls

    public init(bundleID: String, calls: KeychainCalls = .system) {
        service = "\(bundleID).github-token"
        accessGroup = "\(Self.teamID).\(bundleID)"
        self.calls = calls
    }

    enum Keychain: CaseIterable { case dataProtection, login }

    func query(_ keychain: Keychain, account: String? = nil) -> [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
        ]
        if let account { query[kSecAttrAccount as String] = account }
        if keychain == .dataProtection {
            query[kSecUseDataProtectionKeychain as String] = true
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
    }

    func attributes(_ keychain: Keychain, token: String, account: String) -> [String: Any] {
        var item = query(keychain, account: account)
        item[kSecValueData as String] = Data(token.utf8)
        item[kSecAttrLabel as String] = "PR Nanny GitHub token"
        if keychain == .dataProtection {
            item[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        }
        return item
    }

    public func token(for account: String) throws -> String? {
        switch try read(.dataProtection, account: account) {
        case .found(let token):
            return token
        case .missingEntitlement:
            if case .found(let token) = try read(.login, account: account) { return token }
            return nil
        case .notFound:
            // A token saved before the app had the entitlement: move it over.
            guard case .found(let token) = try read(.login, account: account) else { return nil }
            if (try? write(.dataProtection, token: token, account: account)) == true {
                _ = calls.delete(query(.login))
            }
            return token
        }
    }

    public func save(_ token: String, for account: String) throws {
        if try write(.dataProtection, token: token, account: account) {
            _ = calls.delete(query(.login))
            return
        }
        guard try write(.login, token: token, account: account) else {
            throw TokenStoreError(status: errSecMissingEntitlement)
        }
    }

    public func deleteAll() throws {
        for keychain in Keychain.allCases {
            let status = calls.delete(query(keychain))
            guard [errSecSuccess, errSecItemNotFound, errSecMissingEntitlement].contains(status) else {
                throw TokenStoreError(status: status)
            }
        }
    }

    private enum ReadResult { case found(String), notFound, missingEntitlement }

    private func read(_ keychain: Keychain, account: String) throws -> ReadResult {
        var request = query(keychain, account: account)
        request[kSecReturnData as String] = true
        request[kSecMatchLimit as String] = kSecMatchLimitOne
        let (status, data) = calls.copyData(request)
        switch status {
        case errSecSuccess:
            guard let data, let token = String(data: data, encoding: .utf8) else {
                throw TokenStoreError(status: errSecDecode)
            }
            return .found(token)
        case errSecItemNotFound: return .notFound
        case errSecMissingEntitlement: return .missingEntitlement
        default: throw TokenStoreError(status: status)
        }
    }

    /// Replaces every token in `keychain` with this one. False when the
    /// keychain needs an entitlement the app does not have.
    private func write(_ keychain: Keychain, token: String, account: String) throws -> Bool {
        let cleared = calls.delete(query(keychain))
        if cleared == errSecMissingEntitlement { return false }
        guard cleared == errSecSuccess || cleared == errSecItemNotFound else {
            throw TokenStoreError(status: cleared)
        }
        let status = calls.add(attributes(keychain, token: token, account: account))
        if status == errSecMissingEntitlement { return false }
        guard status == errSecSuccess else { throw TokenStoreError(status: status) }
        return true
    }
}

/// The token a PR Nanny 0.1.x helper saved, returned by `legacyToken`.
public struct LegacyToken: Decodable, Equatable, Sendable {
    public let account: String
    public let token: String
}

/// The helper operations `TokenHandoff` needs. `HelperClient` implements them.
public protocol HelperTokenAPI: Sendable {
    func provideToken(account: String, token: String) throws -> Snapshot
    /// Nil when there is no 0.1.x item for the configured account.
    func legacyToken() throws -> LegacyToken?
    func deleteLegacyToken() throws
}

/// Moves the PAT between the app's keychain and the helper's memory.
public enum TokenHandoff {
    public enum Outcome: Equatable, Sendable {
        /// The account does not use a PAT; any saved token was removed.
        case notNeeded
        /// The saved token was handed to the helper.
        case provided
        /// A 0.1.x token was moved into the app's keychain and the old item deleted.
        case migrated
        /// No token anywhere; Settings asks the user to connect again.
        case missing
        /// Something failed; `message` is safe to show.
        case failed(String)
    }

    public static let readFailed =
        "PR Nanny could not read its GitHub token from the keychain. Unlock the keychain and try again."
    public static let saveFailed =
        "PR Nanny is connected, but could not save the token to the keychain. Connect again after the next launch."
    public static let migrationFailed =
        "PR Nanny could not move its GitHub token into the keychain. It will try again at the next launch."

    /// Runs once each time the helper starts.
    public static func start(settings: AppSettings, store: any TokenStore, helper: any HelperTokenAPI) -> Outcome {
        guard settings.authMethod == "pat", !settings.account.isEmpty else {
            try? store.deleteAll()
            return .notNeeded
        }
        let saved: String?
        do {
            saved = try store.token(for: settings.account)
        } catch {
            return .failed(readFailed)
        }
        if let saved {
            do {
                _ = try helper.provideToken(account: settings.account, token: saved)
                return .provided
            } catch {
                return .failed(readFailed)
            }
        }
        // First launch after 0.1.x: the helper reads the old item and loads it.
        let legacy: LegacyToken?
        do {
            legacy = try helper.legacyToken()
        } catch {
            return .failed(migrationFailed)
        }
        guard let legacy else { return .missing }
        do {
            try store.save(legacy.token, for: legacy.account)
        } catch {
            return .failed(migrationFailed)
        }
        guard (try? helper.deleteLegacyToken()) != nil else { return .failed(migrationFailed) }
        return .migrated
    }

    /// After `connectPAT` succeeded: keep the token for the next launch. Nil on success.
    public static func saveConnected(token: String, settings: AppSettings, store: any TokenStore) -> String? {
        guard settings.authMethod == "pat", !settings.account.isEmpty else { return nil }
        do {
            try store.save(token, for: settings.account)
            return nil
        } catch {
            return saveFailed
        }
    }

    /// After switching to the GitHub CLI: the PAT is no longer needed.
    public static func forgetUnused(settings: AppSettings, store: any TokenStore) {
        if settings.authMethod != "pat" { try? store.deleteAll() }
    }
}
