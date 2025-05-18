// swift-tools-version: 5.10
// The swift-tools-version declares the minimum version of Swift required to build this package.

import PackageDescription

let package = Package(
	name: "CakeAgent",
	platforms: [
		.macOS(.v13)
	],
	products: [
		// Products define the executables and libraries a package produces, making them visible to other packages.
		.executable(name: "cakeagent", targets: ["CakeAgent"]),
		.executable(name: "testagent", targets: ["TestAgent"]),
		.library(name: "CakeAgentLib", targets: ["CakeAgentLib"]),
	],
	dependencies: [
		.package(url: "https://github.com/apple/swift-argument-parser", from: "1.3.1"),
		.package(url: "https://github.com/grpc/grpc-swift.git", from: "1.25.0"),
		.package(url: "https://github.com/apple/swift-nio.git", from: "2.82.0"),
		.package(url: "https://github.com/apple/swift-protobuf.git", from: "1.29.0"),
		.package(url: "https://github.com/apple/swift-log.git", from: "1.6.2"),
		.package(url: "https://github.com/groue/Semaphore", from: "0.0.8"),
		.package(url: "https://github.com/Fred78290/swift-nio-portforwarding.git", .upToNextMajor(from: "0.2.2")),
	],
	targets: [
		.target(name: "CakeAgentLib",
			dependencies: [
				.product(name: "GRPC", package: "grpc-swift"),
				.product(name: "Semaphore", package: "Semaphore"),
				.product(name: "NIOPortForwarding", package: "swift-nio-portforwarding")
			],
			path: "darwin/Sources/CakeAgentLib",
			swiftSettings: [
				.define("XTRACE")
			]),
		.executableTarget(name: "CakeAgent",
			dependencies: [
				.product(name: "ArgumentParser", package: "swift-argument-parser"),
				.product(name: "GRPC", package: "grpc-swift"),
				.product(name: "Logging", package: "swift-log"),
				.product(name: "NIOCore", package: "swift-nio"),
				.product(name: "NIOEmbedded", package: "swift-nio"),
				.product(name: "NIOFoundationCompat", package: "swift-nio"),
				.product(name: "NIOHTTP1", package: "swift-nio"),
				.product(name: "NIOPosix", package: "swift-nio"),
				.product(name: "NIOTLS", package: "swift-nio"),
				.target(name: "CakeAgentLib"),
			],
			path: "darwin/Sources/CakeAgent"),
		.executableTarget(name: "TestAgent",
			dependencies: [
				.product(name: "ArgumentParser", package: "swift-argument-parser"),
				.product(name: "GRPC", package: "grpc-swift"),
				.product(name: "Logging", package: "swift-log"),
				.product(name: "NIOCore", package: "swift-nio"),
				.product(name: "NIOEmbedded", package: "swift-nio"),
				.product(name: "NIOFoundationCompat", package: "swift-nio"),
				.product(name: "NIOHTTP1", package: "swift-nio"),
				.product(name: "NIOPosix", package: "swift-nio"),
				.product(name: "NIOTLS", package: "swift-nio"),
				.target(name: "CakeAgentLib"),
			],
			path: "darwin/Sources/TestAgent"),
	]
)
