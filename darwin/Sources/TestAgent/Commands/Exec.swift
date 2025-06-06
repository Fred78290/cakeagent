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

	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? {
		try? CakeAgentClientInterceptorFactory(inputHandle: FileHandle.standardInput)
	}

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())

		if arguments.count < 1 {
			throw ValidationError("At least one argument is required")
		}
	}

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
		let command = self.arguments.remove(at: 0)
		//#if TRACE
		//FileHandle.standardError.write("Executing command: \(command) with arguments: \(self.arguments), enter to continue\n".data(using: .utf8)!)
		//_ = readLine()
		//#endif
		Foundation.exit(try await CakeAgentHelper(on: on, client: client).exec(command: command, arguments: self.arguments, callOptions: callOptions))
	}
}
