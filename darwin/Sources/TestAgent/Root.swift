import ArgumentParser
import Foundation
import GRPC
import CakeAgentLib
import NIOCore
import NIOPosix
import NIOSSL

let cakerSignature = "com.aldunelabs.cakeagent"

class GrpcError: Error {
	let code: Int
	let reason: String

	init(code: Int, reason: String) {
		self.code = code
		self.reason = reason
	}
}

protocol GrpcParsableCommand: AsyncParsableCommand {
	var options: CakeAgentClientOptions { get }
	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? { get }
	var retries: ConnectionBackoff.Retries { get }

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws
}

extension GrpcParsableCommand {
	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? { nil }
	var retries: ConnectionBackoff.Retries { .unlimited }

	func execute(command: GrpcParsableCommand) async throws {
		let command = command
		let group = MultiThreadedEventLoopGroup(numberOfThreads: 1)

		do {
			let grpcClient = try self.options.createClient(on: group, retries: self.retries, interceptors: self.interceptors)

			do {
				try await command.run(on: group,
				                      client: grpcClient,
				                      callOptions: CallOptions(timeLimit: TimeLimit.timeout(TimeAmount.seconds(options.timeout))))

				try! await grpcClient.close()
			} catch {
				try! await grpcClient.close()
				throw error
			}

			try! await group.shutdownGracefully()
		} catch {
			try! await group.shutdownGracefully()
			throw error
		}
	}

	public mutating func run() async throws {
		do {
			try await self.execute(command: self)
		} catch {
			if let err = error as? GrpcError {
				fputs("\(err.reason)\n", stderr)

				Foundation.exit(Int32(err.code))
			}
			// Handle any other exception, including ArgumentParser's ones
			Self.exit(withError: error)
		}
	}
}

@main
struct Root: AsyncParsableCommand {
	@OptionGroup var options: CakeAgentClientOptions

	static var configuration = CommandConfiguration(commandName:"testagent", abstract: "Test Shell",
	                                                version: "1.0.0",
	                                                subcommands: [
	                                                	Shell.self,
	                                                	Exec.self,
	                                                	Run.self,
	                                                	Shutdown.self,
	                                                	Infos.self,
	                                                ])
	public static func getHome(asSystem: Bool = false) throws -> URL {
		let cakeHomeDir: URL

		if asSystem {
			let paths = NSSearchPathForDirectoriesInDomains(.applicationSupportDirectory, .systemDomainMask, true)
			var applicationSupportDirectory = URL(fileURLWithPath: paths.first!, isDirectory: true)

			applicationSupportDirectory = URL(fileURLWithPath: cakerSignature,
			                                  isDirectory: true,
			                                  relativeTo: applicationSupportDirectory)
			try FileManager.default.createDirectory(at: applicationSupportDirectory, withIntermediateDirectories: true)

			cakeHomeDir = applicationSupportDirectory
		} else if let customHome = ProcessInfo.processInfo.environment["CAKE_HOME"] {
			cakeHomeDir = URL(fileURLWithPath: customHome)
		} else {
			cakeHomeDir = FileManager.default
				.homeDirectoryForCurrentUser
				.appendingPathComponent(".cake", isDirectory: true)
		}

		try FileManager.default.createDirectory(at: cakeHomeDir, withIntermediateDirectories: true)

		return cakeHomeDir
	}

	static func getDefaultServerAddress() throws -> String {
		if let cakeListenAddress = ProcessInfo.processInfo.environment["CAKEAGENT_LISTEN_ADDRESS"] {
			return cakeListenAddress
		} else {
			var cakeHomeDir = try Self.getHome(asSystem: false)

			cakeHomeDir.append(path: ".cakeagent.sock")

			return "unix://\(cakeHomeDir.absoluteURL.path())"
		}
	}

	public static func main() async throws {
		// Set line-buffered output for stdout
		setlinebuf(stdout)

		// Parse and run command
		do {
			var command = try parseAsRoot()

			if var asyncCommand = command as? AsyncParsableCommand {
				try await asyncCommand.run()
			} else {
				try command.run()
			}
		} catch {
			if error is CancellationError == false {

				if let err = error as? GrpcError {
					fputs("\(err.reason)\n", stderr)

					Foundation.exit(Int32(err.code))
				}
				// Handle any other exception, including ArgumentParser's ones
				Self.exit(withError: error)
			}
		}
	}
}
