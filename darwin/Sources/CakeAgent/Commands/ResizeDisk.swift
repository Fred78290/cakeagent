import Foundation
import ArgumentParser
import CakeAgentLib

struct ResizeDisk: ParsableCommand {
	static let configuration: CommandConfiguration = CommandConfiguration(abstract: "Resize MacOS disk")

	@Option(name: [.customLong("log-level")], help: "Log level")
	var logLevel: Logger.LogLevel = .info

	func validate() throws {
		Logger.setLevel(self.logLevel)
	}

	func run() throws {
		try ResizeHandler.resizeDisk()
	}
}
