import MenubarKit
import PRNannyNative
import SwiftUI

/// The 340pt menu-bar panel: status, layout switch, pull request lists, menu.
struct PanelView: View {
    @EnvironmentObject private var model: AppModel
    @State private var tab: PRCategory = .authored
    @State private var folded: Set<PRCategory> = []
    /// Where the version footer reads the version; renders pass a fixture bundle.
    var versionBundle: Bundle = .main

    var body: some View {
        MenubarPanel {
            header
            if !model.actionError.isEmpty {
                PanelNotice(model.actionError, symbol: "xmark.octagon")
            } else if let snapshot = model.snapshot, ConnectionPresentation.showsBanner(snapshot) {
                PanelNotice(ConnectionPresentation.statusMessage(snapshot), symbol: "info.circle")
            }
            content
            MenuSection(quitTitle: "Quit PR Nanny", quitHelp: "Stops monitoring until you open PR Nanny again.") {
                OpenSettingsButton("Settings…")
            }
            VersionFooter(bundle: versionBundle)
        }
    }

    private var header: some View {
        StatusHeader(model.status) {
            if model.helper == .running, model.snapshot != nil {
                RefreshButton(
                    isRefreshing: model.busy || model.snapshot?.refreshing == true, title: "Refresh pull requests"
                ) {
                    model.refresh()
                }
            }
        }
    }

    @ViewBuilder private var content: some View {
        switch model.helper {
        case .alreadyRunning, .unavailable:
            PrimaryActionButton("Try Again") { model.retry() }
        case .starting, .running:
            if let snapshot = model.snapshot {
                if ConnectionPresentation.authRequired(snapshot.connection) {
                    authRequired(snapshot)
                } else {
                    lists(snapshot)
                }
            } else {
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Loading pull requests…").foregroundStyle(.secondary)
                }
                .padding(.horizontal, 4)
            }
        }
    }

    private func authRequired(_ snapshot: Snapshot) -> some View {
        let reconnect = ConnectionPresentation.needsReconnect(snapshot.connection)
        return VStack(alignment: .leading, spacing: 8) {
            Text(reconnect ? "Reconnect to GitHub" : "Connect to GitHub").font(.headline)
            Text(
                verbatim: snapshot.message.isEmpty
                    ? (reconnect
                        ? "Your connection needs attention to keep PRs up to date."
                        : "Connect your account to see your open pull requests here.")
                    : snapshot.message
            )
            .font(.callout).foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
            OpenSettingsButton("Open Settings").buttonStyle(ProminentButtonStyle(fullWidth: true))
        }
        .padding(12)
        .groupedTile()
    }

    private func lists(_ snapshot: Snapshot) -> some View {
        TimelineView(.periodic(from: .now, by: 15)) { context in
            let now = context.date
            let layout = PanelLayout(settings: snapshot.settings)
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    Picker("Pull request layout", selection: layoutBinding(layout)) {
                        ForEach(PanelLayout.allCases, id: \.self) { Text($0.label).tag($0) }
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    .fixedSize()
                    .disabled(model.busy)
                    Spacer()
                    Text(ConnectionPresentation.countLabel(snapshot.pullRequests.count))
                        .font(.caption).foregroundStyle(.secondary)
                }
                if layout == .tabs {
                    Picker("Pull request categories", selection: $tab) {
                        ForEach(PRCategory.allCases) { category in
                            Text("\(category.tabLabel) \(category.filter(snapshot.pullRequests, now: now).count)")
                                .tag(category)
                        }
                    }
                    .pickerStyle(.segmented)
                    .labelsHidden()
                    BoundedScrollView(maxHeight: 380) {
                        rows(tab.filter(snapshot.pullRequests, now: now), category: tab, now: now, empty: tab.emptyText)
                    }
                } else if snapshot.pullRequests.isEmpty {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("You’re all caught up.")
                        Text("New pull requests will show up here.").font(.caption).foregroundStyle(.secondary)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .groupedTile()
                } else {
                    BoundedScrollView(maxHeight: 420) {
                        VStack(alignment: .leading, spacing: 8) {
                            ForEach(PRCategory.allCases) { category in
                                section(category, items: category.filter(snapshot.pullRequests, now: now), now: now)
                            }
                        }
                    }
                }
            }
        }
    }

    private func section(_ category: PRCategory, items: [PullRequest], now: Date) -> some View {
        let isFolded = folded.contains(category)
        return VStack(alignment: .leading, spacing: 2) {
            Button {
                if isFolded { folded.remove(category) } else { folded.insert(category) }
            } label: {
                HStack(spacing: 4) {
                    Image(systemName: isFolded ? "chevron.right" : "chevron.down").font(.caption2.weight(.bold))
                    Text(category.sectionTitle)
                    Text("\(items.count)").foregroundStyle(.secondary)
                    Spacer()
                }
                .font(.callout.weight(.semibold))
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 4)
            .accessibilityValue(isFolded ? "Collapsed" : "Expanded")
            if !isFolded {
                rows(items, category: category, now: now, empty: "None right now")
            }
        }
    }

    @ViewBuilder
    private func rows(_ items: [PullRequest], category: PRCategory, now: Date, empty: String) -> some View {
        if items.isEmpty {
            Text(empty)
                .font(.callout).foregroundStyle(.secondary)
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .groupedTile()
        } else {
            VStack(spacing: 0) {
                ForEach(items) { pr in
                    PRRowView(
                        pr: pr, now: now, busy: model.busy,
                        onOpen: { model.open(prID: pr.id) },
                        onBell: { model.setBell(pr, enabled: !pr.bell) })
                }
            }
            .groupedTile()
        }
    }

    private func layoutBinding(_ current: PanelLayout) -> Binding<PanelLayout> {
        Binding(get: { current }, set: { if $0 != current { model.setLayout($0) } })
    }
}

/// One pull request: the whole row opens it; the bell is a separate control.
/// Every API string is shown with `Text(verbatim:)`, never as Markdown.
struct PRRowView: View {
    let pr: PullRequest
    let now: Date
    let busy: Bool
    let onOpen: () -> Void
    let onBell: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 4) {
            Button(action: onOpen) {
                VStack(alignment: .leading, spacing: 2) {
                    HStack(spacing: 4) {
                        let parts = pr.repositoryParts
                        (Text(verbatim: parts.owner).foregroundColor(.secondary) + Text(verbatim: parts.name).bold())
                            .lineLimit(1).truncationMode(.middle)
                        Text(verbatim: "#\(pr.number)").foregroundStyle(.secondary)
                        Spacer(minLength: 4)
                        Text(verbatim: relativeAge(pr.createdAt, now: now)).foregroundStyle(.secondary)
                            .help(pr.createdAt.formatted(date: .abbreviated, time: .shortened))
                    }
                    .font(.caption)
                    Text(verbatim: pr.title).lineLimit(2).font(.callout)
                    HStack(spacing: 10) {
                        StatusDot(label: pr.approvalLabel, color: pr.approved ? .green : .secondary)
                        StatusDot(label: pr.ci.label, color: ciColor)
                        if pr.merged { Text("Merged").foregroundStyle(.purple) }
                    }
                    .font(.caption)
                }
                .padding(.vertical, 4)
            }
            .buttonStyle(MenuRowButtonStyle())
            .accessibilityLabel(pr.accessibilityOpenLabel)
            Button(action: onBell) {
                Image(systemName: pr.bell ? "bell.fill" : "bell")
                    .foregroundStyle(pr.bell ? Color.accentColor : Color.secondary)
                    .frame(width: 24, height: 24)
            }
            .buttonStyle(.borderless)
            .disabled(busy)
            .help(pr.bell ? "Notifications on" : "Notifications off")
            .accessibilityLabel(pr.bellLabel)
            .padding(.top, 6)
            .padding(.trailing, 6)
        }
    }

    private var ciColor: Color {
        switch pr.ci {
        case .passed: .green
        case .notPassed: .red
        case .running: .orange
        case .notAvailable: .secondary
        }
    }
}

private struct StatusDot: View {
    let label: String
    let color: Color

    var body: some View {
        HStack(spacing: 4) {
            Circle().fill(color).frame(width: 6, height: 6).accessibilityHidden(true)
            Text(label).foregroundStyle(.secondary)
        }
    }
}
