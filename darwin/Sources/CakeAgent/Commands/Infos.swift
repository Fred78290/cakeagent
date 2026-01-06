import Foundation
import ArgumentParser
import Logging
import CakeAgentLib

struct Infos: ParsableCommand {
	static let configuration: CommandConfiguration = CommandConfiguration(abstract: "Display MacOS system information")

	@Option(name: [.customLong("log-level")], help: "Log level")
	var logLevel: Logging.Logger.Level = .info

	@Flag(help: "Output format: text or json")
	var format: Format = .text

	func validate() throws {
		Logger.setLevel(self.logLevel)
	}

	func run() throws {
		let result = CakeAgentLib.InfoReply(info: InfosHandler.infos(cpuWatcher: .init()))

		print(self.format.render(result))
	}
}
