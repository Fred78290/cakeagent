import ArgumentParser
import Foundation
import GRPC
import CakeAgentLib
import NIO

final class Exec: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "exec", abstract: "Test execute")

	@OptionGroup var options: Root.Options

	@Argument(help: "Command to execute")
	var arguments: [String]

	func validate() throws {
		try self.options.validate()
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {

		let response = try await client.execute(Cakeagent_ExecuteRequest.with { req in
			if isatty(FileHandle.standardInput.fileDescriptor) == 0{
				req.input = FileHandle.standardInput.readDataToEndOfFile()
			}

			req.command = self.arguments.joined(separator: " ")
		}).response.get()

		if response.hasError {
			FileHandle.standardError.write(response.error)
		}
		
		if response.hasOutput {
			FileHandle.standardOutput.write(response.output)
		}

		Foundation.exit(response.exitCode)
	}
}
