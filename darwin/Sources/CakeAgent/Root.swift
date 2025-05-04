import ArgumentParser
import Foundation

@main
struct Root: AsyncParsableCommand {
	static let configuration = CommandConfiguration(commandName:"cakeagent", abstract: "Cake agent running in guest",
	                                                version: CI.version,
	                                                subcommands: [
	                                                	Run.self,
	                                                	Service.self,
	                                                	ResizeDisk.self,
	                                                ])

	public static func main() async throws {
		// Ensure the default SIGINT handled is disabled,
		// otherwise there's a race between two handlers
		signal(SIGINT, SIG_IGN)
		// Handle cancellation by Ctrl+C ourselves
		let task = withUnsafeCurrentTask { $0 }!
		let sigintSrc = DispatchSource.makeSignalSource(signal: SIGINT)
		sigintSrc.setEventHandler {
			task.cancel()
		}
		sigintSrc.activate()

		// Set line-buffered output for stdout
		setlinebuf(stdout)

		// Parse and run command
		do {
			var command = try Self.parseAsRoot()

			if var asyncCommand = command as? AsyncParsableCommand {
				try await asyncCommand.run()
			} else {
				try command.run()
			}
		} catch {
			/*if let err = error as? GrpcError {
				fputs("\(err.reason)\n", stderr)

				Foundation.exit(Int32(err.code))
			}*/
			// Handle any other exception, including ArgumentParser's ones
			Self.exit(withError: error)
		}
	}
}
