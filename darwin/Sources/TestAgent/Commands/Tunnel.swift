import ArgumentParser
import Foundation
import Logging
import NIO
@preconcurrency import GRPC
import CakeAgentLib
import NIOPortForwarding

enum Proto: String, Codable, ExpressibleByArgument {
	case tcp
	case udp
	
	var mapped: MappedPort.Proto {
		switch self {
		case .tcp:
			return .tcp
		case .udp:
			return .udp
		}
	}
}

final class Tunnel: GrpcParsableCommand {
	static var configuration = CommandConfiguration(commandName: "tunnel", abstract: "Test tunnel")

	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? {
		return nil //try? CakeAgentClientInterceptorFactory(inputHandle: FileHandle.standardInput)
	}

	@OptionGroup var options: CakeAgentClientOptions

	@Option(name: [.customLong("remote")], help: "remote address tunnel to")
	var remote: String

	@Option(name: [.customLong("local")], help: "local bind tunnel address")
	var local: String

	@Option(name: [.customLong("proto")], help: "protocol to use")
	var proto: Proto = .tcp

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
		let remoteAddress = try SocketAddress(unixDomainSocketPath: self.remote)
		let bindAddress = try SocketAddress(unixDomainSocketPath: self.local, cleanupExistingSocketFile: true)

		Foundation.exit(try CakeAgentHelper(on: on, client: client).tunnel(bindAddress: bindAddress, remoteAddress: remoteAddress, proto: proto.mapped, callOptions: .init(timeLimit: .none)))
	}
}
