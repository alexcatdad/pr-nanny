import Foundation
import Testing

@testable import PRNannyNative

@Test func framesWithABigEndianLength() throws {
    let frame = try HelperClient.frame(Data("{}".utf8))
    #expect(Array(frame) == [0, 0, 0, 2, 0x7b, 0x7d])
    #expect(throws: HelperClientError.frameTooLarge) {
        try HelperClient.frame(Data(count: HelperClient.maxFrameBytes + 1))
    }
}

@Test func encodesTheVersionedEnvelope() throws {
    let data = try JSONEncoder().encode(RequestEnvelope(id: "c0ffee", op: "setBell", body: ["id": "PR_x"]))
    let object = try #require(try JSONSerialization.jsonObject(with: data) as? [String: Any])
    #expect(object["version"] as? Int == 1)
    #expect(object["id"] as? String == "c0ffee")
    #expect(object["op"] as? String == "setBell")
}

@Test func decodesBodiesAndFixedErrors() throws {
    let ok: HealthReply = try HelperClient.decodeReply(
        Data(#"{"version":1,"id":"a","body":{"status":"ok"}}"#.utf8), requestID: "a")
    #expect(ok.status == "ok")
    let failure = Data(#"{"version":1,"id":"a","error":{"code":"SETTINGS","message":"Unable to save settings."}}"#.utf8)
    #expect(throws: HelperError(code: "SETTINGS", message: "Unable to save settings.")) {
        let _: HealthReply = try HelperClient.decodeReply(failure, requestID: "a")
    }
}

@Test func rejectsMismatchedOrMalformedReplies() {
    let other = Data(#"{"version":1,"id":"b","body":{"status":"ok"}}"#.utf8)
    #expect(throws: HelperClientError.invalidResponse) {
        let _: HealthReply = try HelperClient.decodeReply(other, requestID: "a")
    }
    let future = Data(#"{"version":2,"id":"a","body":{"status":"ok"}}"#.utf8)
    #expect(throws: HelperClientError.invalidResponse) {
        let _: HealthReply = try HelperClient.decodeReply(future, requestID: "a")
    }
    let empty = Data(#"{"version":1,"id":"a"}"#.utf8)
    #expect(throws: HelperClientError.invalidResponse) {
        let _: HealthReply = try HelperClient.decodeReply(empty, requestID: "a")
    }
    #expect(throws: HelperClientError.invalidResponse) {
        let _: HealthReply = try HelperClient.decodeReply(Data("nope".utf8), requestID: "a")
    }
}

@Test func givesAccountConnectionsLongerTimeouts() {
    #expect(HelperClient.timeout(for: "connectPAT") == 35)
    #expect(HelperClient.timeout(for: "connectCLI") == 35)
    #expect(HelperClient.timeout(for: "snapshot") == 5)
}

@Test func reportsAnUnavailableHelper() {
    let client = HelperClient(socketPath: "/tmp/pr-nanny-test-missing-\(getpid()).sock")
    #expect(throws: HelperClientError.unavailable) { try client.health() }
    let long = HelperClient(socketPath: "/tmp/" + String(repeating: "x", count: 200))
    #expect(throws: HelperClientError.invalidSocketPath) { try long.health() }
}

@Test func developmentBundlesUseTheirOwnDataDirectory() {
    let support = URL(filePath: "/Users/fixture/Library/Application Support")
    let release = AppIdentity(bundleID: "com.alexcatdad.pr-nanny", applicationSupport: support)
    #expect(!release.isDevelopment)
    #expect(release.socketPath == "/Users/fixture/Library/Application Support/pr-nanny/pr-nanny.sock")
    #expect(release.helperArguments.isEmpty)
    let dev = AppIdentity(bundleID: "com.alexcatdad.pr-nanny.dev", applicationSupport: support)
    #expect(dev.isDevelopment)
    #expect(dev.helperArguments == ["-data-dir", "/Users/fixture/Library/Application Support/pr-nanny-dev"])
}

@Test func retriesAnotherCopyForTheLockWaitThenStops() {
    #expect(HelperExit.decide(status: 3, attempt: 0) == .restart(after: 1))
    #expect(HelperExit.decide(status: 3, attempt: 3, waitingForLock: 15) == .restart(after: 8))
    #expect(HelperExit.decide(status: 3, attempt: HelperExit.maxAttempts + 2, waitingForLock: 39) == .restart(after: 8))
    #expect(HelperExit.decide(status: 3, attempt: 6, waitingForLock: HelperExit.lockWait) == .alreadyRunning)
}

@Test func restartsWithBackoff() {
    #expect(HelperExit.decide(status: 1, attempt: 0) == .restart(after: 1))
    #expect(HelperExit.decide(status: 1, attempt: 3) == .restart(after: 8))
    #expect(HelperExit.decide(status: 1, attempt: 5) == .restart(after: 30))
    #expect(HelperExit.decide(status: 1, attempt: HelperExit.maxAttempts) == .givenUp)
}
