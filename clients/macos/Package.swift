// swift-tools-version: 5.9
import PackageDescription
let package = Package(
    name: "APTEXMacClient",
    platforms: [.macOS(.v13)],
    products: [.executable(name: "APTEXClient", targets: ["APTEXClient"])],
    targets: [
        .target(name: "APTEXClientCore"),
        .executableTarget(name: "APTEXClient", dependencies: ["APTEXClientCore"]),
        .testTarget(name: "APTEXClientCoreTests", dependencies: ["APTEXClientCore"])
    ]
)
