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

final class Tunnel: ParsableCommand {
	static var configuration = CommandConfiguration(commandName: "tunnel", abstract: "Test tunnel", subcommands: [Local.self, Remote.self])
	final class Local: GrpcParsableCommand {
		static var configuration = CommandConfiguration(commandName: "local", abstract: "Local tunnel")

		@OptionGroup(title: "Client agent options")
		var options: CakeAgentClientOptions
		
		@Option(name: [.customLong("remote")], help: "remote address tunnel to")
		var remote: String
		
		@Option(name: [.customLong("local")], help: "local bind tunnel address")
		var local: String
		
		@Option(name: [.customLong("proto")], help: "protocol to use")
		var proto: Proto = .tcp
		
		var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? {
			return nil //try? CakeAgentClientInterceptorFactory(inputHandle: FileHandle.standardInput)
		}
		
		func validate() throws {
			try self.options.validate(try Root.getDefaultServerAddress())
		}
		
		func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
			let remoteAddress = try SocketAddress(unixDomainSocketPath: self.remote)
			let bindAddress = try SocketAddress(unixDomainSocketPath: self.local, cleanupExistingSocketFile: true)
			
			Foundation.exit(try CakeAgentHelper(on: on, client: client).tunnel(bindAddress: bindAddress, remoteAddress: remoteAddress, proto: proto.mapped, callOptions: .init(timeLimit: .none)))
		}
	}
	
	final class Remote: GrpcParsableCommand {
		static var configuration = CommandConfiguration(commandName: "remote", abstract: "Remote tunnel")

		@OptionGroup(title: "Client agent options")
		var options: CakeAgentClientOptions

		@Option(name: [.customLong("publish"), .customShort("p")], help: ArgumentHelp("Optional forwarded port for VM, syntax like docker", discussion: "value is like host:guest/(tcp|udp|both)", valueName: "value"))
		public var forwardedPorts: [TunnelAttachement] = []

		var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? {
			return nil //try? CakeAgentClientInterceptorFactory(inputHandle: FileHandle.standardInput)
		}
		
		func validate() throws {
			try self.options.validate(try Root.getDefaultServerAddress())
			try self.forwardedPorts.validate()

			if self.forwardedPorts.isEmpty {
				throw ValidationError("No forwarded ports specified")
			}
		}
		
		func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
			try await CakeAgentPortForwardingServer.createPortForwardingServer(group: on, cakeAgentClient: client, name: "localhost", remoteAddress: "127.0.0.1", forwardedPorts: self.forwardedPorts, dynamicPortForwarding: false).get()
		}
	}
}
