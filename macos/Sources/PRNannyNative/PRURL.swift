import Foundation

/// Swift mirror of Go `app.ValidPRURL` (internal/app/url.go): a canonical
/// GitHub.com pull request URL only. The helper already validated it; the app
/// checks again before handing anything to `NSWorkspace.open`.
///
/// It is deliberately at least as strict as Go: no user info, port, escapes,
/// query, fragment, whitespace, control or non-ASCII characters, and the exact
/// lowercase `https://github.com` origin.
public enum PRURL {
    private static let prefix = "https://github.com"
    private static let pathPattern = "^/[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9_.-]+/pull/[1-9][0-9]*$"

    public static func isValid(_ raw: String) -> Bool {
        guard raw.utf8.count <= 2048, raw.hasPrefix(prefix + "/") else { return false }
        guard raw.unicodeScalars.allSatisfy({ $0.isASCII && $0.value > 0x20 && $0.value < 0x7f }) else {
            return false
        }
        guard !raw.contains(where: { "?#%@\\".contains($0) }) else { return false }
        let pathPart = String(raw.dropFirst(prefix.count))
        guard pathPart.range(of: pathPattern, options: .regularExpression) != nil else { return false }
        let segments = pathPart.split(separator: "/", omittingEmptySubsequences: false)
        return segments.count == 5 && segments[2] != "." && segments[2] != ".."
    }

    /// A `URL` only for valid input.
    public static func url(_ raw: String) -> URL? {
        isValid(raw) ? URL(string: raw) : nil
    }
}
