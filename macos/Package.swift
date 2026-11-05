// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "PRNanny",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PRNanny", targets: ["PRNanny"])
    ],
    dependencies: [
        // menubar-kit main (no tag yet). SwiftPM cannot resolve four-part CalVer tags, so pin the commit.
        .package(
            url: "https://github.com/alexcatdad/menubar-kit.git",
            revision: "7403cb02d14946d9d92f4d7b1dd441316745a622")
    ],
    targets: [
        // All testable logic lives in PRNannyNative; the app target is UI glue.
        .target(
            name: "PRNannyNative",
            dependencies: [.product(name: "MenubarKit", package: "menubar-kit")]),
        .executableTarget(
            name: "PRNanny",
            dependencies: ["PRNannyNative", .product(name: "MenubarKit", package: "menubar-kit")]),
        .testTarget(name: "PRNannyNativeTests", dependencies: ["PRNannyNative"]),
    ]
)
