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
	var options: Root.Options { get }

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws
}

extension GrpcParsableCommand {

	public mutating func run() async throws {
		do {
			try await self.options.execute(command: self)
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
	@OptionGroup var options: Self.Options

	static var configuration = CommandConfiguration(commandName:"testagent", abstract: "Test Shell",
	                                                version: "1.0.0",
	                                                subcommands: [
	                                                	ShellAsync.self,
	                                                	Exec.self,
	                                                	Infos.self,
	                                                ])

	struct Options: ParsableArguments {

		@Option(help: "Connection timeout in seconds")
		var timeout: Int64 = 120

		@Flag(name: [.customLong("insecure")], help: "don't use TLS")
		var insecure: Bool = false

		@Option(name: [.customLong("connect")], help: "connect to address")
		var address: String = try! Root.getDefaultServerAddress()

		@Option(name: [.customLong("ca-cert")], help: "CA TLS certificate")
		var caCert: String?

		@Option(name: [.customLong("tls-cert")], help: "Client TLS certificate")
		var tlsCert: String?

		@Option(name: [.customLong("tls-key")], help: "Client private key")
		var tlsKey: String?

		func execute(command: GrpcParsableCommand) async throws {
			let command = command
			let group = MultiThreadedEventLoopGroup(numberOfThreads: 1)

			do {
				let connection = try Root.createClient(on: group,
				                                       listeningAddress: URL(string: self.address),
				                                       caCert: self.caCert,
				                                       tlsCert: self.tlsCert,
				                                       tlsKey: self.tlsKey)

				do {
					let grpcClient = Cakeagent_AgentNIOClient(channel: connection)
					try await command.run(on: group, client: grpcClient,
					                      callOptions: CallOptions(timeLimit: TimeLimit.timeout(TimeAmount.seconds(self.timeout))))

					try! await connection.close().get()
				} catch {
					try! await connection.close().get()
					throw error
				}

			} catch {
				try! await group.shutdownGracefully()
				throw error
			}
		}

		mutating func validate() throws {
			if self.insecure {
				self.caCert = nil
				self.tlsCert = nil
				self.tlsKey = nil
			} else {
				if let tlsCert, let tlsKey, let caCert {
					if FileManager.default.fileExists(atPath: tlsCert) == false {
						throw ValidationError("TLS certificate \(tlsCert) does not exist")
					}

					if FileManager.default.fileExists(atPath: tlsKey) == false {
						throw ValidationError("TLS key \(tlsKey) does not exist")
					}

					if FileManager.default.fileExists(atPath: caCert) == false {
						throw ValidationError("CA certificate \(caCert) does not exist")
					}
				}
			}
		}
	}

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

	static func createClient(on: MultiThreadedEventLoopGroup,
	                         listeningAddress: URL?,
	                         caCert: String?,
	                         tlsCert: String?,
	                         tlsKey: String?) throws -> ClientConnection {
		if let listeningAddress = listeningAddress {
			let target: ConnectionTarget

			if listeningAddress.scheme == "unix" {
				target = ConnectionTarget.unixDomainSocket(listeningAddress.path())
			} else if listeningAddress.scheme == "tcp" {
				target = ConnectionTarget.hostAndPort(listeningAddress.host ?? "127.0.0.1", listeningAddress.port ?? 5000)
			} else {
				throw GrpcError(
					code: -1,
					reason:
					"unsupported address scheme: \(String(describing: listeningAddress.scheme))")
			}

			var clientConfiguration = ClientConnection.Configuration.default(target: target, eventLoopGroup: on)

			if let tlsCert = tlsCert, let tlsKey = tlsKey {
				let tlsCert = try NIOSSLCertificate(file: tlsCert, format: .pem)
				let tlsKey = try NIOSSLPrivateKey(file: tlsKey, format: .pem)
				let trustRoots: NIOSSLTrustRoots

				if let caCert: String = caCert {
					trustRoots = .certificates([try NIOSSLCertificate(file: caCert, format: .pem)])
				} else {
					trustRoots = NIOSSLTrustRoots.default
				}

				clientConfiguration.tlsConfiguration = GRPCTLSConfiguration.makeClientConfigurationBackedByNIOSSL(
					certificateChain: [.certificate(tlsCert)],
					privateKey: .privateKey(tlsKey),
					trustRoots: trustRoots,
					certificateVerification: .noHostnameVerification)
			}

			return ClientConnection(configuration: clientConfiguration)
		}

		throw GrpcError(code: -1, reason: "connection address must be specified")
	}

	public static func main() async throws {
		// Ensure the default SIGINT handled is disabled,
		// otherwise there's a race between two handlers
		signal(SIGINT, SIG_IGN)
		// Handle cancellation by Ctrl+C ourselves
		let task = withUnsafeCurrentTask { $0 }!
		let sigintSrc = DispatchSource.makeSignalSource(signal: SIGINT)
		sigintSrc.setEventHandler {
			task.cancel()
		}
		sigintSrc.activate()

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
