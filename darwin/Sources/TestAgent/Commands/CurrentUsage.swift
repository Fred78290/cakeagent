import ArgumentParser
import Foundation
import GRPC
import CakeAgentLib
import NIO
import TextTable

final class CurrentUsage: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "current", abstract: "Test currentUsage")

	@OptionGroup var options: CakeAgentClientOptions

	@Argument(help: "Frequency")
	var frequency: Int32 = 1

	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? {
		try? CakeAgentClientInterceptorFactory(inputHandle: FileHandle.standardInput)
	}

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func render<T>(style: TextTableStyle.Type = Style.grid, uppercased: Bool = true, _ data: [T]) -> String {
		if data.count == 0 {
			return ""
		}
		let table = TextTable<T> { (item: T) in
			return Mirror(reflecting: item).children.enumerated()
				.map { (_, element) in
					let label = element.label ?? "<unknown>"
					return Column(title: uppercased ? label.uppercased() : label, value: element.value)
				}
		}

		return table.string(for: data, style: style)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
	}

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
		try CakeAgentHelper(on: on, client: client).currentUsage(frequency: self.frequency, callOptions: callOptions) { usage in
			print(self.render(usage.cpuInfos.cores))
		}
	}
}
