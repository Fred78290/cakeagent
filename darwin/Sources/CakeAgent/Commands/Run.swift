import ArgumentParser
import Foundation
import GRPC
import NIOSSL
import NIOCore
import NIOPosix
//import Synchronization
import Logging

struct Run: ParsableCommand {
	static let configuration: CommandConfiguration = CommandConfiguration(abstract: "cake agent running")

	@Option(name: [.customLong("listen"), .customShort("l")], help: "Listen on address")
	var address: String?

	@Option(name: [.customLong("ca-cert"), .customShort("c")], help: "CA TLS certificate")
	var caCert: String?

	@Option(name: [.customLong("tls-cert"), .customShort("t")], help: "Server TLS certificate")
	var tlsCert: String?

	@Option(name: [.customLong("tls-key"), .customShort("k")], help: "Server private key")
	var tlsKey: String?

	private func getServerAddress() throws -> String {
		if let address = self.address {
			return address
		} else {
			return "vsock://any:5000"
		}
	}

	static func createServer(on: MultiThreadedEventLoopGroup,
	                         listeningAddress: URL?,
	                         caCert: String?,
	                         tlsCert: String?,
	                         tlsKey: String?) throws -> EventLoopFuture<Server> {

		if let listeningAddress = listeningAddress {
			let target: ConnectionTarget

			if listeningAddress.scheme == "vsock" {
				let cid: VsockAddress.ContextID

				if let host = listeningAddress.host {
					switch host {
					case "host":
						cid = .host
					case "any":
						cid = .any
					default:
						cid = .init(Int(host)!)
					}
				} else {
					cid = .any
				}

				target = ConnectionTarget.vsockAddress(VsockAddress(cid: cid, port: .init(listeningAddress.port ?? 5000)))
			} else if listeningAddress.scheme == "unix" {
				target = ConnectionTarget.unixDomainSocket(listeningAddress.path())
			} else if listeningAddress.scheme == "tcp" {
				target = ConnectionTarget.hostAndPort(listeningAddress.host ?? "127.0.0.1", listeningAddress.port ?? 5000)
			} else {
				throw ServiceError("unsupported listening address scheme: \(String(describing: listeningAddress.scheme))")
			}

			var serverConfiguration = Server.Configuration.default(target: target,
			                                                       eventLoopGroup: on,
			                                                       serviceProviders: [CakeAgentProvider(group: on)])

			if let tlsCert = tlsCert, let tlsKey = tlsKey {
				let tlsCert = try NIOSSLCertificate(file: tlsCert, format: .pem)
				let tlsKey = try NIOSSLPrivateKey(file: tlsKey, format: .pem)
				let trustRoots: NIOSSLTrustRoots

				if let caCert: String = caCert {
					trustRoots = .certificates([try NIOSSLCertificate(file: caCert, format: .pem)])
				} else {
					trustRoots = NIOSSLTrustRoots.default
				}

				serverConfiguration.tlsConfiguration = GRPCTLSConfiguration.makeServerConfigurationBackedByNIOSSL(
					certificateChain: [.certificate(tlsCert)],
					privateKey: .privateKey(tlsKey),
					trustRoots: trustRoots,
					certificateVerification: CertificateVerification.none,
					requireALPN: false)
			}

			return Server.start(configuration: serverConfiguration)
		}

		throw ServiceError("connection address must be specified")
	}

	mutating func run() throws {
		let logger: Logger = Logging.Logger(label: "com.aldunelabs.cakeagent")
		let group = MultiThreadedEventLoopGroup(numberOfThreads: System.coreCount)

		defer {
			try! group.syncShutdownGracefully()
		}

		let listenAddress = try self.getServerAddress()

		logger.info("Start agent on \(listenAddress)")

		// Start the server and print its address once it has started.
		let server = try Self.createServer(on: group,
		                                   listeningAddress: URL(string: listenAddress),
		                                   caCert: self.caCert,
		                                   tlsCert: self.tlsCert,
		                                   tlsKey: self.tlsKey).wait()

		signal(SIGINT, SIG_IGN)

		let sigintSrc: any DispatchSourceSignal = DispatchSource.makeSignalSource(signal: SIGINT)

		sigintSrc.setEventHandler {
			try? server.close().wait()
		}

		sigintSrc.activate()

		// Wait on the server's `onClose` future to stop the program from exiting.
		try server.onClose.wait()

		logger.info("Agent stopped")
	}
}
