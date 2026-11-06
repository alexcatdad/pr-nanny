import Foundation

/// Where this bundle keeps its data. Development bundles (`.dev`) use their own
/// directory so they never contend for the installed app's instance lock.
public struct AppIdentity: Equatable, Sendable {
    public static let bundleID = "com.alexcatdad.pr-nanny"
    public static let helperExecutable = "pr-nanny-helper"

    public let isDevelopment: Bool
    public let dataDirectory: URL

    public init(bundleID: String?, applicationSupport: URL) {
        isDevelopment = bundleID == "\(Self.bundleID).dev"
        dataDirectory = applicationSupport.appending(
            path: isDevelopment ? "pr-nanny-dev" : "pr-nanny", directoryHint: .isDirectory)
    }

    public static func current(bundle: Bundle = .main) -> AppIdentity {
        let support =
            FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(filePath: NSHomeDirectory()).appending(path: "Library/Application Support")
        return AppIdentity(bundleID: bundle.bundleIdentifier, applicationSupport: support)
    }

    public var socketPath: String { dataDirectory.appending(path: "pr-nanny.sock").path }
    public var displayName: String { "PR Nanny" }

    /// Arguments for the helper. The default data directory needs none.
    public var helperArguments: [String] {
        isDevelopment ? ["-data-dir", dataDirectory.path] : []
    }

    public static func helperURL(bundle: Bundle = .main) -> URL {
        bundle.bundleURL.appending(path: "Contents/Helpers/\(helperExecutable)")
    }
}

/// How the supervisor reacts when the helper exits.
public enum HelperExit: Equatable, Sendable {
    /// Another PR Nanny process still holds the instance lock (exit status 3)
    /// after `lockWait` seconds of retries. Not restarted.
    case alreadyRunning
    /// Restart after this many seconds.
    case restart(after: TimeInterval)
    /// Too many consecutive failures; wait for the user to retry.
    case givenUp

    public static let alreadyRunningStatus: Int32 = 3
    public static let maxAttempts = 6
    /// A helper from a crashed or upgraded app can hold the lock for about
    /// 35 seconds while it finishes a request and notices its parent is gone.
    public static let lockWait: TimeInterval = 40

    /// Exponential backoff 1, 2, 4 … 30 seconds, then give up. Exit 3 is
    /// retried with 1, 2, 4, 8, 8 … seconds until `lockWait` has passed since
    /// the first one (`waitingForLock`).
    public static func decide(status: Int32, attempt: Int, waitingForLock: TimeInterval = 0) -> HelperExit {
        if status == alreadyRunningStatus {
            guard waitingForLock < lockWait else { return .alreadyRunning }
            return .restart(after: min(8, pow(2, Double(attempt))))
        }
        guard attempt < maxAttempts else { return .givenUp }
        return .restart(after: min(30, pow(2, Double(attempt))))
    }
}

/// Launches `Contents/Helpers/pr-nanny-helper` as a child process. The helper
/// watches its parent and exits when the app goes away.
public struct HelperSupervisor: Sendable {
    public let identity: AppIdentity
    public init(identity: AppIdentity) { self.identity = identity }

    public func launch(bundle: Bundle = .main, onTermination: @escaping @Sendable (Int32) -> Void) throws -> Process {
        let executable = AppIdentity.helperURL(bundle: bundle)
        guard FileManager.default.isExecutableFile(atPath: executable.path) else { throw SupervisorError.helperMissing }
        let process = Process()
        process.executableURL = executable
        process.arguments = identity.helperArguments
        process.standardInput = FileHandle.nullDevice
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        process.terminationHandler = { onTermination($0.terminationStatus) }
        try process.run()
        return process
    }
}

public enum SupervisorError: Error, LocalizedError {
    case helperMissing
    public var errorDescription: String? {
        "PR Nanny's background helper is missing. Reinstall PR Nanny to repair it."
    }
}
