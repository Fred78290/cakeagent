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
		
		if arguments.count < 1 {
			throw ValidationError("At least one argument is required")
		}
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {
		let command = self.arguments.remove(at: 0)

		Foundation.exit(try CakeAgentHelper(on: on, client: client).exec(command: command, arguments: self.arguments, callOptions: callOptions))
	}
}
