import AppKit
import MenubarKit
import PRNannyNative
import SwiftUI
import UserNotifications

struct PRNannyApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    private let trayIcon = TrayIconLoader.load(resourcesDirectory: Bundle.main.resourceURL, baseName: "PRNannyTrayIcon")

    var body: some Scene {
        MenuBarExtra {
            PanelView()
                .environmentObject(delegate.model)
                .onAppear { delegate.model.panelVisible = true }
                .onDisappear { delegate.model.panelVisible = false }
        } label: {
            TrayLabelView(model: delegate.model, icon: trayIcon)
        }
        .menuBarExtraStyle(.window)

        Settings {
            SettingsView().environmentObject(delegate.model)
        }
    }
}

/// Observes the model so the glyph's badge and VoiceOver label follow the status.
struct TrayLabelView: View {
    @ObservedObject var model: AppModel
    let icon: NSImage?

    var body: some View {
        TrayLabel(
            icon: icon, fallbackSymbol: "checkmark.shield", status: model.status,
            appName: model.identity.displayName, isDevelopment: model.identity.isDevelopment)
    }
}

/// Starts the helper at launch, stops it at quit, and opens the pull request
/// when the user clicks one of its notifications.
@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    let model = AppModel()

    func applicationDidFinishLaunching(_ notification: Notification) {
        UNUserNotificationCenter.current().delegate = self
        model.start()
    }

    func applicationWillTerminate(_ notification: Notification) {
        model.stop()
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse
    ) async {
        guard response.actionIdentifier == UNNotificationDefaultActionIdentifier else { return }
        // The thread identifier is the PR node ID; the helper resolves it to a validated URL.
        let prID = response.notification.request.content.threadIdentifier
        guard !prID.isEmpty else { return }
        await MainActor.run { model.open(prID: prID) }
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .list]
    }
}

/// A panel row that opens the Settings scene and brings it in front: a
/// menu-bar app is never active on its own.
struct OpenSettingsButton: View {
    let title: String

    var body: some View {
        if #available(macOS 14, *) {
            ModernOpenSettingsButton(title: title)
        } else {
            Button(title) {
                NSApp.activate(ignoringOtherApps: true)
                NSApp.sendAction(Selector(("showSettingsWindow:")), to: nil, from: nil)
            }
        }
    }
}

@available(macOS 14, *)
private struct ModernOpenSettingsButton: View {
    let title: String
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        Button(title) {
            NSApp.activate(ignoringOtherApps: true)
            openSettings()
        }
    }
}
