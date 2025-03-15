import ArgumentParser
@preconcurrency import GRPC
import CakeAgentLib
import Foundation
import NIO

protocol CatchRemoteCloseDelegate {
	func closedByRemote()
}

final class CatchRemoteClose: ChannelInboundHandler {
	typealias InboundIn = Any

	let closedByRemote: () -> Void

	init(closedByRemote:  @escaping @Sendable () -> Void) {
		self.closedByRemote = closedByRemote
	}

	func channelInactive(context: ChannelHandlerContext) {
		context.flush()
		self.closedByRemote()
		context.fireChannelInactive()
	}
}

final class Shell: GrpcParsableCommand {
	static var configuration = CommandConfiguration(commandName: "shell", abstract: "Test shell")

	@OptionGroup var options: CakeAgentClientOptions

	var interceptors: Cakeagent_AgentClientInterceptorFactoryProtocol? {
		Self.interceptorFactory(inputHandle: FileHandle.standardInput)
	}

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {
		Foundation.exit(try await CakeAgentHelper(on: on, client: client).shell(callOptions: .init(timeLimit: .none)))
	}
}
