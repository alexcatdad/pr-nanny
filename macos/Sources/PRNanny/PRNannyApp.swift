import AppKit
import MenubarKit
import PRNannyNative
import SwiftUI

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
final class AppDelegate: NSObject, NSApplicationDelegate {
    let model = AppModel()
    // Kept alive here: the notification center holds its delegate weakly.
    private lazy var responder = MenubarNotificationResponder(presentInForeground: false) { [weak self] click in
        guard let prID = NotificationMapping.prID(for: click) else { return }
        self?.model.open(prID: prID)
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        responder.install()
        model.start()
    }

    func applicationWillTerminate(_ notification: Notification) {
        model.stop()
    }
}
