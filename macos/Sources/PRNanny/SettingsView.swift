import MenubarKit
import PRNannyNative
import SwiftUI

/// The Settings window: GitHub account, refresh interval, notifications, startup, about.
struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @AppStorage(NotificationPreferences.enabledKey) private var notify = false
    @State private var method: AuthMethod?
    @State private var token = ""
    @State private var notice = ""
    @State private var working = false
    @State private var launchAtLogin = false

    var body: some View {
        SettingsForm {
            if let snapshot = model.snapshot {
                account(snapshot)
                refresh(snapshot)
                NotificationSettingsSection(
                    notifier: model.notifier, toggleTitle: "Allow notifications", offersSound: true,
                    enabledKey: NotificationPreferences.enabledKey,
                    soundEnabledKey: NotificationPreferences.soundEnabledKey)
                events(snapshot)
                startup
            } else {
                Text("PR Nanny is starting. Settings appear once its helper is ready.").foregroundStyle(.secondary)
            }
            if !notice.isEmpty {
                Text(notice).font(.callout).foregroundStyle(.secondary).accessibilityAddTraits(.updatesFrequently)
            }
            SettingsAboutSection(repository: URL(string: "https://github.com/alexcatdad/pr-nanny"))
        }
        .disabled(working || model.busy)
        // The kit section owns the toggle and the permission prompt; the helper gates
        // notifications too, so mirror the switch into its settings.
        .onChange(of: notify) { enabled in mirrorNotifications(enabled) }
        .task { launchAtLogin = model.loginItemEnabled }
    }

    // MARK: Sections

    private func account(_ snapshot: Snapshot) -> some View {
        let selected = method ?? AuthMethod(rawValue: snapshot.settings.authMethod) ?? .gh
        return Section {
            LabeledContent("Status") {
                Text(verbatim: accountLine(snapshot)).foregroundStyle(.secondary)
            }
            if !snapshot.message.isEmpty {
                Text(verbatim: snapshot.message).font(.callout).foregroundStyle(.secondary)
            }
            Picker(
                "Authentication method",
                selection: Binding(
                    get: { selected },
                    set: {
                        method = $0
                        token = ""
                    })
            ) {
                ForEach(AuthMethod.allCases) { Text($0.label).tag($0) }
            }
            .pickerStyle(.segmented)
            if selected == .pat {
                SecureField("Personal access token", text: $token, prompt: Text(verbatim: "ghp_••••••••••••"))
                    .onSubmit(submitToken)
                HStack {
                    Text("Stored securely in your macOS Keychain.").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button("Connect", action: submitToken).disabled(token.isEmpty)
                }
            } else {
                HStack {
                    Text("Uses your `gh auth login` session for github.com.").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button("Connect GitHub CLI", action: connectCLI)
                }
            }
        } header: {
            Text("GitHub account")
        }
    }

    private func refresh(_ snapshot: Snapshot) -> some View {
        Section {
            Picker(
                "Check GitHub every",
                selection: Binding(
                    get: { snapshot.settings.intervalSeconds },
                    set: { seconds in
                        var settings = snapshot.settings
                        settings.intervalSeconds = seconds
                        save(settings)
                    })
            ) {
                ForEach(RefreshInterval.all) { Text($0.label).tag($0.seconds) }
            }
            Text("Shorter intervals use more of your GitHub rate limit.").font(.caption).foregroundStyle(.secondary)
        } header: {
            Text("Refresh")
        }
    }

    private func events(_ snapshot: Snapshot) -> some View {
        Section {
            ForEach(EventToggle.all) { event in
                Toggle(
                    event.label,
                    isOn: Binding(
                        get: { snapshot.settings.events[keyPath: event.keyPath] },
                        set: { value in
                            var settings = snapshot.settings
                            settings.events[keyPath: event.keyPath] = value
                            save(settings)
                        })
                )
                .disabled(!notify)
            }
            SettingsNote(SettingsNotice.hint)
        } header: {
            Text("Notify me about")
        }
    }

    private var startup: some View {
        Section {
            Toggle("Launch at login", isOn: $launchAtLogin)
                .onChange(of: launchAtLogin) { enabled in
                    guard enabled != model.loginItemEnabled else { return }
                    Task {
                        working = true
                        notice = await model.setLaunchAtLogin(enabled)
                        launchAtLogin = model.loginItemEnabled
                        working = false
                    }
                }
        } header: {
            Text("Startup")
        }
    }

    // MARK: Actions

    private func accountLine(_ snapshot: Snapshot) -> String {
        switch snapshot.connection {
        case "connected":
            "Connected as \(snapshot.settings.account.isEmpty ? "GitHub user" : snapshot.settings.account)"
        case "error": "Connection needs attention"
        default: ConnectionPresentation.connectionLine(snapshot)
        }
    }

    private func save(_ settings: AppSettings) {
        Task {
            notice = ""
            if !(await model.save(settings)) { notice = SettingsNotice.saveFailed }
        }
    }

    private func mirrorNotifications(_ enabled: Bool) {
        guard let current = model.snapshot?.settings, current.notifications != enabled else { return }
        Task {
            if !(await model.save(SettingsRules.withNotifications(enabled, current: current))) {
                notice = SettingsNotice.saveFailed
            }
        }
    }

    private func submitToken() {
        // Clear the field before the helper answers; the token lives only in this call.
        let submitted = token
        token = ""
        guard !submitted.isEmpty else { return }
        Task {
            working = true
            notice = ""
            let connected = await model.connectPAT(submitted)
            notice = connected ? SettingsNotice.patConnected : SettingsNotice.patFailed
            if connected { method = nil }
            working = false
        }
    }

    private func connectCLI() {
        Task {
            working = true
            notice = ""
            let connected = await model.connectCLI()
            notice = connected ? SettingsNotice.cliConnected : SettingsNotice.cliFailed
            if connected { method = nil }
            working = false
        }
    }
}
