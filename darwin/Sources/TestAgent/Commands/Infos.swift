import ArgumentParser
import Foundation
import NIO
import GRPC
import CakeAgentLib
import SwiftProtobuf

final class Infos: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "infos", abstract: "Test infos")

	@OptionGroup var options: CakeAgentClientOptions

	@Flag(help: "Output format: text or json")
	var format: Format = .text

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
		print(self.format.render(try CakeAgentHelper(on: on, client: client).info(callOptions: callOptions)))
	}
}
