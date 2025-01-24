import ArgumentParser
@preconcurrency import GRPC
import CakeAgentLib
import Foundation
import NIO

protocol CatchRemoteCloseDelegate {
	func closedByRemote()
}

final class CatchRemoteClose: ChannelInboundHandler {
	typealias InboundIn = Any

	let closedByRemote: () -> Void

	init(closedByRemote:  @escaping @Sendable () -> Void) {
		self.closedByRemote = closedByRemote
	}

	func channelInactive(context: ChannelHandlerContext) {
		context.flush()
		self.closedByRemote()
		context.fireChannelInactive()
	}
}

final class ShellAsync: GrpcParsableCommand {
	static var configuration = CommandConfiguration(commandName: "shell", abstract: "Test shell")

	@OptionGroup var options: Root.Options

	func validate() throws {
		try self.options.validate()
	}

	func run(on: EventLoopGroup, client: Cakeagent_AgentNIOClient, callOptions: CallOptions?) async throws {		
		var shellStream: BidirectionalStreamingCall<Cakeagent_ShellMessage, Cakeagent_ShellResponse>?
		var pipeChannel: NIOAsyncChannel<ByteBuffer, ByteBuffer>?

		shellStream = client.shell(callOptions: .init(), handler: { response in
			if response.format == .stdout {
				FileHandle.standardOutput.write(response.datas)
			} else if response.format == .stderr {
				FileHandle.standardError.write(response.datas)
			} else {
				if let channel = pipeChannel {					
					_ = channel.channel.close()
				}
			}
		})

		if let stream = shellStream {
			pipeChannel = try await stream.subchannel.flatMapThrowing { streamChannel in
				/*try streamChannel.pipeline.syncOperations.addHandler(CatchRemoteClose {
				 	print("closed by remote")
				 	_ = stream.sendEnd()
				 })*/

				return Task {
					return try await NIOPipeBootstrap(group: on)
						//.takingOwnershipOfDescriptors(input: FileHandle.standardInput.fileDescriptor, output: FileHandle.standardOutput.fileDescriptor) { pipeChannel in
						.takingOwnershipOfDescriptor(input: FileHandle.standardInput.fileDescriptor) { pipeChannel in
							/*try! pipeChannel.pipeline.syncOperations.addHandler(CatchRemoteClose {
							 	print("closed by remote")
							 	_ = stream.sendEnd()
							 })*/

							pipeChannel.closeFuture.whenComplete { _ in
								_ = stream.sendEnd()
							}

							return pipeChannel.eventLoop.makeCompletedFuture {
								try NIOAsyncChannel<ByteBuffer, ByteBuffer>(wrappingChannelSynchronously: pipeChannel)
							}
						}
				}
			}.get().value

			try await pipeChannel!.executeThenClose { inbound, outbound in

				do {
					for try await buffer: ByteBuffer in inbound {
						let message = Cakeagent_ShellMessage.with {
							$0.datas = Data(buffer: buffer)
						}

						_ = stream.sendMessage(message)
					}
				} catch {
					if error is CancellationError == false {
						guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
							FileHandle.standardError.write("error: \(error)\n".data(using: .utf8)!)
							return
						}
					}
				}
				_ = stream.sendEnd()
			}
		}
	}
}
