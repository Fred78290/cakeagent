import ArgumentParser
import Foundation

struct Version: ParsableCommand {
	static let configuration = CommandConfiguration(commandName: "version", abstract: "Display the version information")
	
	func run() throws {		
		print(CI.version)
	}
}