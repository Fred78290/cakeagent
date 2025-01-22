import ArgumentParser
@preconcurrency import GRPC
import CakeAgentLib
import Foundation
import NIO

final class Shell : GrpcParsableCommand {
	static var configuration = CommandConfiguration(commandName: "shell", abstract: "Test shell")

	@OptionGroup var options: Root.Options

	func validate() throws {
		try self.options.validate()
	}

	func restoreTermios(_ fd: Int32, termios: UnsafePointer<termios>!) {
		tcsetattr(fd, TCSANOW, termios)
	}

	func setupTermios(_ fd: Int32) -> termios {
		let TTY_CTRL_OPTS: tcflag_t = tcflag_t(CS8 | CLOCAL | CREAD)
		let TTY_INPUT_OPTS: tcflag_t = tcflag_t(IGNPAR)
		let TTY_OUTPUT_OPTS:tcflag_t = 0
		let TTY_LOCAL_OPTS:tcflag_t = 0

		var termios_ = termios()
		tcgetattr(STDIN_FILENO, &termios_)

		cfmakeraw(&termios_)

		// This attempts to replicate the behaviour documented for cfmakeraw in
		// the termios(3) manpage.
		termios_.c_iflag = TTY_INPUT_OPTS
		termios_.c_oflag = TTY_OUTPUT_OPTS
		termios_.c_lflag = TTY_LOCAL_OPTS
		termios_.c_cflag = TTY_CTRL_OPTS
		termios_.c_cc.16 = 1 // Darwin.VMIN
		termios_.c_cc.17 = 0 // Darwin.VTIME

		tcsetattr(STDIN_FILENO, TCSANOW, &termios_)

		return termios_
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {		
		let stream = client.shell(callOptions: .init(), handler: { response in
			FileHandle.standardOutput.write(response.datas)
		})

		/*let channel = NIOPipeBootstrap(group: stream.eventLoop)
		 .takingOwnershipOfDescriptors(input: FileHandle.standardInput.fileDescriptor, output: FileHandle.standardOutput.fileDescriptor).flatMap { pipeChannel in
		 	stream.subchannel.flatMap { streamChannel in
		 		let (ours, theirs) = GlueHandler.matchedPair()

		 		return streamChannel.pipeline.addHandler(ours).flatMap {
		 			pipeChannel.eventLoop.makeSucceededFuture {
		 				pipeChannel.pipeline.addHandler(theirs)
		 			}
		 		}
		 	}
		 }*/

		let future = stream.subchannel.flatMap { streamChannel in
			NIOPipeBootstrap(group: streamChannel.eventLoop)
				.channelOption(ChannelOptions.autoRead, value: true)
				.takingOwnershipOfDescriptors(input: FileHandle.standardInput.fileDescriptor, output: FileHandle.standardOutput.fileDescriptor).flatMap { pipeChannel in
					let (ours, theirs) = GlueHandler.matchedPair()

					return streamChannel.pipeline.addHandler(ours).flatMap { Void in
						pipeChannel.pipeline.addHandler(theirs).flatMap { Void in
							return pipeChannel.eventLoop.makeSucceededFuture {
								return pipeChannel
							}
						}
					}
				}
		}

		let channel = try await future.get()()

		channel.closeFuture.whenComplete { _ in
			_ = stream.sendEnd()
		}

		while true {
			channel.read()
		}
	}
}
