import ArgumentParser
import Foundation
import Logging
import NIO
@preconcurrency import GRPC
import CakeAgentLib


final class Tunnel: GrpcParsableCommand {
	static var configuration = CommandConfiguration(commandName: "tunnel", abstract: "Test tunnel")

	@OptionGroup var options: CakeAgentClientOptions

	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? = nil

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	@Option(name: [.customLong("remote")], help: "remote address tunnel to")
	var remote: String

	@Option(name: [.customLong("local")], help: "local bind tunnel address")
	var local: String

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
		Foundation.exit(try CakeAgentHelper(on: on, client: client).tunnel(protocallOptions: .init(timeLimit: .none)))
	}
}