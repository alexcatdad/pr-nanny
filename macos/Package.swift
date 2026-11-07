// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "PRNanny",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PRNanny", targets: ["PRNanny"])
    ],
    dependencies: [
        // menubar-kit v2026.11.07.1. SwiftPM cannot resolve four-part CalVer tags, so pin the tag's commit.
        .package(
            url: "https://github.com/alexcatdad/menubar-kit.git",
            revision: "4027df8aaafd84a9c3dc46571477c76b5d74921d")
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
        // Hosts the app's SwiftUI views to check their layout.
        .testTarget(
            name: "PRNannyTests",
            dependencies: ["PRNanny", "PRNannyNative", .product(name: "MenubarKit", package: "menubar-kit")]),
    ]
)
