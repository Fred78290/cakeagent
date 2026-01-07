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
		.package(url: "https://github.com/apple/swift-argument-parser", from: "1.7.0"),
		.package(url: "https://github.com/grpc/grpc-swift.git", revision: "5770e231585cb68388a6ae37ce36c4d8dbd46d90"),
		.package(url: "https://github.com/apple/swift-nio.git", from: "2.92.1"),
		.package(url: "https://github.com/apple/swift-protobuf.git", from: "1.33.3"),
		.package(url: "https://github.com/apple/swift-log.git", from: "1.8.0"),
		.package(url: "https://github.com/groue/Semaphore", from: "0.0.8"),
		.package(url: "https://github.com/Fred78290/swift-nio-portforwarding.git", .upToNextMajor(from: "0.2.9")),
		.package(url: "https://github.com/cfilipov/TextTable", branch: "master"),
	],
	targets: [
		.target(name: "CakeAgentLib",
			dependencies: [
				.product(name: "TextTable", package: "TextTable"),
				.product(name: "GRPC", package: "grpc-swift"),
				.product(name: "Semaphore", package: "Semaphore"),
				.product(name: "NIOPortForwarding", package: "swift-nio-portforwarding"),
				.product(name: "Logging", package: "swift-log"),
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
				.product(name: "TextTable", package: "TextTable"),
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
				.product(name: "TextTable", package: "TextTable"),
				.target(name: "CakeAgentLib"),
			],
			path: "darwin/Sources/TestAgent"),
	]
)
