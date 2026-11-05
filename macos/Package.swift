// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "PRNanny",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PRNanny", targets: ["PRNanny"])
    ],
    dependencies: [
        // menubar-kit v2026.11.05.2. SwiftPM cannot resolve four-part CalVer tags, so pin the tag's commit.
        .package(
            url: "https://github.com/alexcatdad/menubar-kit.git",
            revision: "ec879dcd4c33638c90f5f5be0a8cd21c5eb75665")
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
