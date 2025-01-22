import ArgumentParser
import Foundation
import NIO
import GRPC
import CakeAgentLib
import SwiftProtobuf

final class Infos: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "infos", abstract: "Test infos")

	@OptionGroup var options: Root.Options

	func validate() throws {
		try self.options.validate()
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {
		let response = client.info(.init(), callOptions: callOptions)

		let json = try await response.response.get().jsonString()
		print(json)
	}
}