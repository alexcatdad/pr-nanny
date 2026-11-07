// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "PRNanny",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PRNanny", targets: ["PRNanny"])
    ],
    dependencies: [
        // menubar-kit v2026.11.07.2. SwiftPM cannot resolve four-part CalVer tags, so pin the tag's commit.
        .package(
            url: "https://github.com/alexcatdad/menubar-kit.git",
            revision: "cf3666e8e1a822930c7f4c3f64309de5951b1c54")
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
