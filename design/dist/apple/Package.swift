// swift-tools-version:5.9
//
// Conduit Design — Swift Package
//
// Tokens and SwiftUI extensions generated from design/tokens.yaml by
// cmd/design-tokens. The generator writes Tokens.swift alongside this
// manifest; this Package.swift wires it into a Swift Package so plugin
// developers can `import ConduitDesign` and match the host app exactly.
import PackageDescription

let package = Package(
    name: "ConduitDesign",
    platforms: [
        .macOS(.v13),
        .iOS(.v16),
        .watchOS(.v9),
        .visionOS(.v1),
    ],
    products: [
        .library(
            name: "ConduitDesign",
            targets: ["ConduitDesign"]
        ),
    ],
    targets: [
        .target(
            name: "ConduitDesign",
            path: ".",
            exclude: ["Package.swift", "README.md"],
            sources: ["Tokens.swift"]
        ),
    ]
)
