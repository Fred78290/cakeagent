import ArgumentParser
import Foundation
import GRPC
import CakeAgentLib
import NIO

final class Exec: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "exec", abstract: "Test execute")

	@OptionGroup var options: CakeAgentClientOptions

	@Argument(help: "Command to execute")
	var arguments: [String]

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {
		Foundation.exit(try await CakeAgentHelper(on: on, client: client).exec(arguments: self.arguments, callOptions: callOptions))
	}
}
