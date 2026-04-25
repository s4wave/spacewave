// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "SpacewaveHelper",
    platforms: [.macOS(.v11)],
    targets: [
        .target(
            name: "SpacewaveUpdateSupport",
            path: "Sources/SpacewaveUpdateSupport"
        ),
        .executableTarget(
            name: "SpacewaveHelper",
            dependencies: ["SpacewaveUpdateSupport"],
            path: "Sources/SpacewaveHelper"
        ),
        .executableTarget(
            name: "SpacewaveHelperPrivileged",
            dependencies: ["SpacewaveUpdateSupport"],
            path: "Sources/SpacewaveHelperPrivileged"
        ),
    ]
)
