import Foundation
import PRNannyNative

/// Process entry. `--smoke-test` checks the bundle headlessly (the release
/// validation step): it starts the bundled helper in a throwaway data
/// directory, asks for its health and version over the socket, stops it and
/// exits. Every other launch is the menu-bar app.
@main
enum Entry {
    static func main() {
        if CommandLine.arguments.contains("--smoke-test") { exit(smokeTest()) }
        PRNannyApp.main()
    }

    static func smokeTest() -> Int32 {
        func fail(_ message: String) -> Int32 {
            FileHandle.standardError.write(Data("PRNanny smoke test: \(message)\n".utf8))
            return 1
        }
        // A short path: unix socket paths are limited to 104 bytes.
        let directory = URL(filePath: "/tmp").appending(path: "prn-smoke-\(getpid())")
        defer { try? FileManager.default.removeItem(at: directory) }
        do {
            try FileManager.default.createDirectory(
                at: directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
        } catch {
            return fail("cannot create a data directory")
        }
        let process = Process()
        process.executableURL = AppIdentity.helperURL()
        process.arguments = ["-data-dir", directory.path]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        do { try process.run() } catch { return fail("the bundled helper did not start") }
        defer { if process.isRunning { process.terminate() } }

        let client = HelperClient(socketPath: directory.appending(path: "pr-nanny.sock").path)
        let deadline = Date().addingTimeInterval(10)
        while Date() < deadline {
            if (try? client.health())?.status == "ok" { break }
            Thread.sleep(forTimeInterval: 0.1)
        }
        guard let version = try? client.version(), version.protocol == HelperClient.protocolVersion else {
            return fail("the helper did not answer")
        }
        let app = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "unknown"
        let _: EmptyBody? = try? client.call("stop", body: EmptyBody())
        process.waitUntilExit()
        print("PR Nanny \(app): helper \(version.version) protocol \(version.protocol) ok")
        return 0
    }
}
