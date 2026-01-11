import ArgumentParser
import Foundation
import GRPC
import CakeAgentLib
import NIO
import TextTable

final class CurrentUsage: GrpcParsableCommand {
	static var configuration: CommandConfiguration = CommandConfiguration(commandName: "current", abstract: "Test currentUsage")

	@OptionGroup(title: "Client agent options")
	var options: CakeAgentClientOptions

	@Argument(help: "Frequency")
	var frequency: Int32 = 1

	@Flag(help: "Output format: text or json")
	var format: Format = .text

	@Flag(help: "Use async method")
	var async: Bool = false

	var interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? {
		try? CakeAgentClientInterceptorFactory(inputHandle: FileHandle.standardInput)
	}

	func validate() throws {
		try self.options.validate(try Root.getDefaultServerAddress())
	}

	func run(on: EventLoopGroup, client: CakeAgentClient, callOptions: CallOptions?) async throws {
		struct CpuInfo: Sendable, Codable {
			var totalUsagePercent: Double = 0
			var user: Double = 0
			var system: Double = 0
			var idle: Double = 0
			var iowait: Double = 0
			var irq: Double = 0
			var softirq: Double = 0
			var steal: Double = 0
			var guest: Double = 0
			var guestNice: Double = 0

			init(from infos: Cakeagent_CakeAgent.InfoReply.CpuInfo) {
				self.totalUsagePercent = infos.totalUsagePercent
				self.user = infos.user
				self.system = infos.system
				self.idle = infos.idle
				self.iowait = infos.iowait
				self.irq = infos.irq
				self.softirq = infos.softirq
				self.steal = infos.steal
				self.guest = infos.guest
				self.guestNice = infos.guestNice
			}
		}

		let cakeHelper = CakeAgentHelper(on: on, client: client)

		func printUsage(_ currentUsage: CakeAgent.CurrentUsageReply) {
			print("\u{001B}[2J\u{001B}[H")
			print(self.format.renderSingle(CpuInfo(from: currentUsage.cpuInfos)))
			print(self.format.renderList(currentUsage.cpuInfos.cores.map(\.agent)))
		}

		if self.async {
			let stream = AsyncThrowingStream.makeStream(of: CakeAgent.CurrentUsageReply.self)

			cakeHelper.currentUsage(frequency: self.frequency, continuation: stream.continuation)

			for try await currentUsage in stream.stream {
				printUsage(currentUsage)
			}
		} else {
			let stream = try cakeHelper.currentUsage(frequency: self.frequency) { currentUsage in
				printUsage(currentUsage)
			}
			
			try await stream.subchannel.get().closeFuture.get()
		}
	}
}
