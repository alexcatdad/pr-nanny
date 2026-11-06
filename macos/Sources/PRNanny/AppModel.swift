import AppKit
import MenubarKit
import PRNannyNative
import ServiceManagement
import SwiftUI

/// Owns the helper process and every call to it. Reads are cached snapshots;
/// only `refresh()` asks the helper to fetch from GitHub.
@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var snapshot: Snapshot?
    @Published private(set) var helper: HelperState = .starting
    @Published private(set) var busy = false
    @Published var actionError = ""
    @Published var panelVisible = false {
        didSet { if panelVisible { syncNow() } }
    }

    let identity = AppIdentity.current()
    let notifier = MenubarNotifier()
    private let client: HelperClient
    private var process: Process?
    private var attempt = 0
    /// When the helper first exited with "already running" in this run of retries.
    private var lockWaitStarted: Date?
    private let tokens: any TokenStore
    /// Whether the saved PAT went to the helper since it last started.
    private var tokenHandedOff = false
    private var stopping = false
    private var startedAt = Date()
    private var cursor = ""
    private var syncTimer: Timer?
    private var notificationTimer: Timer?
    private var syncing = false
    /// Bumped by every action so a slower background read cannot overwrite a newer result.
    private var generation = 0

    init() {
        client = HelperClient(socketPath: identity.socketPath)
        tokens = KeychainTokenStore(bundleID: Bundle.main.bundleIdentifier ?? AppIdentity.bundleID)
    }

    var status: MenubarStatus { ConnectionPresentation.status(helper: helper, snapshot: snapshot) }

    // MARK: Lifecycle

    func start() {
        guard process == nil, !stopping else { return }
        try? FileManager.default.createDirectory(
            at: identity.dataDirectory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        helper = .starting
        startedAt = Date()
        tokenHandedOff = false
        do {
            process = try HelperSupervisor(identity: identity).launch { [weak self] status in
                Task { @MainActor in self?.helperExited(status) }
            }
        } catch {
            helper = .unavailable(error.localizedDescription)
            return
        }
        scheduleTimers()
        syncNow()
    }

    func stop() {
        stopping = true
        syncTimer?.invalidate()
        notificationTimer?.invalidate()
        process?.terminate()
    }

    /// "Try again" after the helper gave up or another copy held the lock.
    func retry() {
        attempt = 0
        lockWaitStarted = nil
        if process == nil { start() } else { syncNow() }
    }

    private func helperExited(_ status: Int32) {
        process = nil
        guard !stopping else { return }
        var waited: TimeInterval = 0
        if status == HelperExit.alreadyRunningStatus {
            let since = lockWaitStarted ?? Date()
            lockWaitStarted = since
            waited = Date().timeIntervalSince(since)
        } else {
            lockWaitStarted = nil
        }
        switch HelperExit.decide(status: status, attempt: attempt, waitingForLock: waited) {
        case .alreadyRunning:
            helper = .alreadyRunning
        case .givenUp:
            helper = .unavailable("The background helper keeps stopping. Try again, or reinstall PR Nanny.")
        case .restart(let delay):
            attempt += 1
            helper = .starting
            DispatchQueue.main.asyncAfter(deadline: .now() + delay) { [weak self] in self?.start() }
        }
    }

    private func scheduleTimers() {
        syncTimer?.invalidate()
        notificationTimer?.invalidate()
        // Local socket reads only. Faster while the panel is open or a refresh runs.
        syncTimer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.tick() }
        }
        notificationTimer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.pollNotifications() }
        }
    }

    private var ticks = 0
    private func tick() {
        ticks += 1
        let every = snapshot == nil || snapshot?.refreshing == true ? 1 : panelVisible ? 5 : 30
        if ticks % every == 0 { syncNow() }
    }

    // MARK: Reads

    func syncNow() {
        guard !syncing, process != nil else { return }
        syncing = true
        let client = client
        let expected = generation
        Task {
            let result = await Self.background { try client.snapshot() }
            syncing = false
            guard expected == generation, !busy else { return }
            switch result {
            case .success(let next):
                attempt = 0
                lockWaitStarted = nil
                helper = .running
                if next != snapshot { snapshot = next }
                if !tokenHandedOff { handOffToken(settings: next.settings) }
            case .failure:
                // The socket appears a moment after launch; only report trouble after a grace period.
                if helper == .running || Date().timeIntervalSince(startedAt) > 10 {
                    helper = .unavailable("PR Nanny's background helper is not responding. Try again in a moment.")
                }
            }
        }
    }

    /// Gives the helper the PAT from the keychain (or migrates the 0.1.x item) once per helper start.
    private func handOffToken(settings: AppSettings) {
        tokenHandedOff = true
        let client = client
        let tokens = tokens
        Task {
            let outcome = await Self.background {
                TokenHandoff.start(settings: settings, store: tokens, helper: client)
            }
            if case .success(.failed(let message)) = outcome { actionError = message }
        }
    }

    private func pollNotifications() {
        guard process != nil, helper == .running else { return }
        let client = client
        let cursor = cursor
        Task {
            guard case .success(let batch) = await Self.background({ try client.nextNotifications(cursor: cursor) })
            else { return }
            self.cursor = batch.cursor
            for entry in batch.notifications {
                await notifier.deliver(NotificationMapping.notification(for: entry))
            }
        }
    }

    // MARK: Actions

    /// Runs one helper action; its snapshot wins over any read still in flight.
    @discardableResult
    func perform(_ action: @escaping @Sendable (HelperClient) throws -> Snapshot) async -> Result<Snapshot, Error> {
        generation += 1
        let mine = generation
        busy = true
        actionError = ""
        let client = client
        let result = await Self.background { try action(client) }
        if mine == generation {
            busy = false
            generation += 1
        }
        if case .success(let next) = result {
            helper = .running
            snapshot = next
        }
        return result
    }

    func refresh() {
        Task {
            let result = await perform { client in
                try client.refresh()
                return try client.snapshot()
            }
            if case .failure = result { actionError = "That action did not complete. Please try again." }
        }
    }

    func setBell(_ pr: PullRequest, enabled: Bool) {
        let id = pr.id
        Task {
            if case .failure = await perform({ try $0.setBell(id: id, enabled: enabled) }) {
                actionError = "That action did not complete. Please try again."
            }
        }
    }

    func setLayout(_ layout: PanelLayout) {
        Task {
            if case .failure = await perform({ try $0.setLayout(layout.rawValue) }) {
                actionError = "That action did not complete. Please try again."
            }
        }
    }

    func open(prID: String) {
        let client = client
        Task {
            let result = await Self.background { try client.resolvePR(id: prID) }
            guard case .success(let raw) = result, let url = PRURL.url(raw) else {
                actionError = "Could not open this pull request."
                return
            }
            NSWorkspace.shared.open(url)
        }
    }

    // MARK: Settings

    func save(_ settings: AppSettings) async -> Bool {
        guard let current = snapshot?.settings else { return false }
        let payload = SettingsRules.preferencesToSave(settings, current: current)
        if case .success = await perform({ try $0.saveSettings(payload) }) { return true }
        return false
    }

    func connectPAT(_ token: String) async -> Bool {
        guard case .success(let next) = await perform({ try $0.connectPAT(token: token) }) else { return false }
        let tokens = tokens
        let saved = await Self.background {
            TokenHandoff.saveConnected(token: token, settings: next.settings, store: tokens)
        }
        if case .success(let message?) = saved { actionError = message }
        return true
    }

    func connectCLI() async -> Bool {
        guard case .success(let next) = await perform({ try $0.connectCLI() }) else { return false }
        let tokens = tokens
        _ = await Self.background { TokenHandoff.forgetUnused(settings: next.settings, store: tokens) }
        return true
    }

    var loginItemEnabled: Bool { SMAppService.mainApp.status == .enabled }

    /// Applies launch at login with `SMAppService.mainApp` and records the preference in the helper.
    func setLaunchAtLogin(_ enabled: Bool) async -> String {
        do {
            if enabled {
                if SMAppService.mainApp.status != .enabled { try SMAppService.mainApp.register() }
            } else if SMAppService.mainApp.status == .enabled {
                try await SMAppService.mainApp.unregister()
            }
        } catch {
            return SMAppService.mainApp.status == .requiresApproval
                ? SettingsNotice.loginItemNeedsApproval : SettingsNotice.loginItemFailed
        }
        if var settings = snapshot?.settings {
            settings.launchAtLogin = enabled
            _ = await save(settings)
        }
        return SMAppService.mainApp.status == .requiresApproval ? SettingsNotice.loginItemNeedsApproval : ""
    }

    nonisolated private static func background<T: Sendable>(_ work: @escaping @Sendable () throws -> T) async -> Result<
        T, Error
    > {
        await Task.detached { Result { try work() } }.value
    }
}
