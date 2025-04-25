import ArgumentParser
import Foundation
import GRPC
import CakeAgentLib
import NIO

final class Shutdown: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "shutdown", abstract: "Test run")

	@OptionGroup var options: CakeAgentClientOptions

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {
		Foundation.exit(try CakeAgentHelper(on: on, client: client).shutdown(callOptions: callOptions).exitCode)
	}
}
