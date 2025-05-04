import Foundation
import ArgumentParser
import Logging

struct ResizeDisk: ParsableCommand {
	static let configuration: CommandConfiguration = CommandConfiguration(abstract: "Resize MacOS disk")

	@Option(name: [.customLong("log-level")], help: "Log level")
	var logLevel: Logging.Logger.Level = .info

	func validate() throws {
		Logger.setLevel(self.logLevel)
	}

	func run() throws {
		try ResizeHandler.resizeDisk()
	}
}
