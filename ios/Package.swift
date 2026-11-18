// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "DeviceTrust",
    platforms: [.iOS(.v15)],
    products: [.library(name: "DeviceTrust", targets: ["DeviceTrust"])],
    targets: [
        // Detection core in C: compiled with xollvm for the binary release.
        .target(name: "DeviceTrustCore", path: "Sources/DeviceTrustCore", publicHeadersPath: "include"),
        .target(name: "DeviceTrust", dependencies: ["DeviceTrustCore"], path: "Sources/DeviceTrust",
                resources: [.copy("PrivacyInfo.xcprivacy")]),
        .testTarget(name: "DeviceTrustTests", dependencies: ["DeviceTrust"], path: "Tests/DeviceTrustTests"),
    ]
)
