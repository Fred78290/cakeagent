import ArgumentParser
import Foundation
import GRPC
import NIO
import NIOPosix
import NIOSSL

public typealias CakeAgentClient = Cakeagent_AgentNIOClient
public typealias CakeAgentInterceptor = Cakeagent_AgentClientInterceptorFactoryProtocol
public struct CakeAgentClientOptions: ParsableArguments {

	@Option(help: "Connection timeout in seconds")
	public var timeout: Int64 = 120

	@Flag(name: [.customLong("insecure")], help: "don't use TLS")
	public var insecure: Bool = false

	@Option(name: [.customLong("connect")], help: "connect to address")
	public var address: String?

	@Option(name: [.customLong("ca-cert")], help: "CA TLS certificate")
	public var caCert: String?

	@Option(name: [.customLong("tls-cert")], help: "Client TLS certificate")
	public var tlsCert: String?

	@Option(name: [.customLong("tls-key")], help: "Client private key")
	public var tlsKey: String?

	public init() {
	}

	public mutating func validate(_ defaultAddress: String? = nil) throws {
		if let address = self.address {
			if address.isEmpty {
				self.address = defaultAddress
			}
		} else {
			self.address = defaultAddress
		}

		guard let address else {
			throw ValidationError("connection address must be specified")
		}

		let listeningAddress = URL(string: address)

		guard let listeningAddress else {
			throw ValidationError("connection address is unparsable")
		}

		if listeningAddress.scheme != "unix" && listeningAddress.scheme != "tcp" && !listeningAddress.isFileURL {
			throw ValidationError("unsupported address scheme: \(String(describing: listeningAddress.scheme))")
		}

		if self.insecure {
			self.caCert = nil
			self.tlsCert = nil
			self.tlsKey = nil
		} else {
			guard let caCert else {
				throw ValidationError("CA certificate is required")
			}

			guard let tlsCert else {
				throw ValidationError("TLS certificate is required")
			}

			guard let tlsKey else {
				throw ValidationError("TLS key is required")
			}

			if !FileManager.default.fileExists(atPath: caCert) {
				throw ValidationError("CA certificate \(caCert) not found")
			}

			if !FileManager.default.fileExists(atPath: tlsCert) {
				throw ValidationError("TLS certificate \(tlsCert) not found")
			}

			if !FileManager.default.fileExists(atPath: tlsKey) {
				throw ValidationError("TLS key \(tlsKey) not found")
			}
		}
	}

	public func createClient(on: EventLoopGroup, retries: ConnectionBackoff.Retries = .unlimited, interceptors: CakeAgentInterceptor? = nil) throws -> CakeAgentClient {
		return try CakeAgentHelper.createClient(on: on,
		                                        listeningAddress: URL(string: self.address!)!,
		                                        connectionTimeout: self.timeout,
		                                        caCert: self.caCert,
		                                        tlsCert: self.tlsCert,
		                                        tlsKey: self.tlsKey,
		                                        retries: retries,
		                                        interceptors: interceptors)
	}
}
