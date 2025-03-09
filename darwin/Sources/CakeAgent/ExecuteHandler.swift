//
//  ExecuteHandler.swift
//  CakeAgent
//
//  Created by Frederic BOLTZ on 09/03/2025.
//
import CakeAgentLib
import GRPC
import NIO
import NIOCore
import SwiftProtobuf
import Foundation
import Logging
import Darwin
import Semaphore
import Synchronization

public typealias CakeAgentExecuteStream = NIOAsyncChannel<ByteBuffer, ByteBuffer>
public typealias Cakeagent_ExecuteRequestStream = GRPCAsyncRequestStream<Cakeagent_ExecuteRequest>
public typealias Cakeagent_ExecuteResponseStream = GRPCAsyncResponseStreamWriter<Cakeagent_ExecuteResponse>

public class ExecuteHandleStream {
	let eventLoop: EventLoop
	let logger: Logger = Logging.Logger(label: "com.aldunelabs.cakeagent")
//	var outBytes: Int = 0
//	var receivedBytes: Int = 0

	init(on: EventLoop) {
		self.eventLoop = on
	}

	func stream2(requestStream: Cakeagent_ExecuteRequestStream, responseStream: Cakeagent_ExecuteResponseStream) async throws {
		let errorPipe = Pipe()
		var exitCode: Int32 = -1
		var process: Process? = nil
		var tty: TTY? = nil

		errorPipe.fileHandleForReading.readabilityHandler = { handle in
			let data = handle.availableData

			if data.isEmpty == false {
				Task {
					let message = Cakeagent_ExecuteResponse.with {
						$0.stderr = data
					}

					_ = try? await responseStream.send(message)
				}
			}
		}

		await withTaskGroup(of: Void.self) { group in
			let (outboundStream, outboundContinuation) = AsyncStream.makeStream(of: Data.self)
			let grp = group
			var outBytes: Int = 0

			group.addTask {
				do {
					var receivedBytes: Int = 0

					for try await request: Cakeagent_ExecuteRequest in requestStream {
						self.logger.debug("Request: \(request)")
						
						if case let .size(size) = request.request {
							tty = TTY(tty: size.rows + size.cols > 0)
							tty!.setTermSize(rows: size.rows, cols: size.cols)
						} else if case let .command(command) = request.request {
							guard let tty: TTY = tty else {
								throw ServiceError("No TTY")
							}
							
							tty.fileHandleForOutput.readabilityHandler = { handle in
								let datas = handle.availableData

								if datas.isEmpty == false {
									self.logger.trace("yield outbound: \(datas.count), [\(String(data: datas, encoding: .utf8) ?? "<unknown>")]")
									outboundContinuation.yield(datas)
								}
							}
							
							// Launch the process
							let cmd: String
							let args: [String]
							let shell: Bool

							switch command.execute {
							case .shell:
								cmd = "/bin/zsh".shell()
								args = ["-i", "-l"]
								shell = true
							case let .command(exec):
								cmd = exec.command
								args = exec.args
								shell = false
							default:
								throw ServiceError("No command")
							}
							
							process = Process()
							print("cmd=\(cmd)")
							print("args=\(args)")
							if let process = process {
								process.executableURL = URL(fileURLWithPath: cmd.lookupPath())
								process.arguments = args
								process.environment = ProcessInfo.processInfo.environment
								process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
								process.standardInput = tty.fileHandleForReading
								process.standardOutput = tty.fileHandleForWriting
								
								if shell {
									process.standardError = tty.fileHandleForWriting
								} else {
									process.standardError = errorPipe.fileHandleForWriting
								}

								process.terminationHandler = { p in
									self.logger.debug("Shell exited")

									if tty.isTTY {
										grp.cancelAll()
									} else {
										outboundContinuation.finish()
									}
								}

								try process.run()
							}

							var grp = grp

							grp.addTask {
								do {
									self.logger.debug("enter outboundStream")
									for try await datas in outboundStream {
										outBytes += datas.count
										self.logger.info("outbound: \(datas.count), outBytes: \(outBytes) [\(String(data: datas, encoding: .utf8) ?? "<unknown>")]")
										
										_ = try await responseStream.send(Cakeagent_ExecuteResponse.with {
											$0.stdout = datas
										})
									}
									self.logger.debug("exit outboundStream, outBytes: \(outBytes)")
								} catch {
									if error is CancellationError == false {
										guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
											self.logger.error("Error reading from shell, \(error)")
											return
										}
									}
								}
							}
						} else if case let .input(datas) = request.request {
							guard let tty = tty else {
								throw ServiceError("No TTY")
							}
							
							receivedBytes += datas.count
							self.logger.info("inbound: \(datas.count), receivedBytes: \(receivedBytes) [\(String(data: datas, encoding: .utf8) ?? "<unknown>")]")

							tty.fileHandleForInput.write(datas)

						} else if case .eof = request.request {
							self.logger.info("received EOF")

							if let tty = tty {
								self.logger.info("EOF")
								tty.eof()
							}
							break
						}
					}
					
				} catch {
					if error is CancellationError == false {
						guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
							self.logger.error("Error reading from shell, \(error)")
							return
						}
					}
				}
				
				self.logger.info("exit task")
			}

			await group.waitForAll()

			if let tty = tty {
				self.logger.info("EOF")
				tty.eof()
			}

			self.logger.info("exit group task")
		}

		self.logger.info("leave group task")

		if let process = process {
			if process.isRunning {
				process.waitUntilExit()
			}

			exitCode = process.terminationStatus
		}

		try await responseStream.send(Cakeagent_ExecuteResponse.with { $0.exitCode = exitCode })
	}
	
	func stream(requestStream: Cakeagent_ExecuteRequestStream, responseStream: Cakeagent_ExecuteResponseStream) async throws {
		let errorPipe = Pipe()
		var exitCode: Int32 = -1
		var process: Process? = nil
		var tty: TTY? = nil
		var outputStream: NIOAsyncChannelOutboundWriter<ByteBuffer>? = nil

		errorPipe.fileHandleForReading.readabilityHandler = { handle in
			let data = handle.availableData

			if data.isEmpty == false {
				Task {
					let message = Cakeagent_ExecuteResponse.with {
						$0.stderr = data
					}

					_ = try? await responseStream.send(message)
				}
			}
		}

		await withTaskGroup(of: Void.self) { group in
			let (channelStream, channelContinuation) = AsyncStream.makeStream(of: CakeAgentExecuteStream.self)
			let (outboundStream, outboundContinuation) = AsyncStream.makeStream(of: NIOAsyncChannelOutboundWriter<ByteBuffer>.self)
			let grp = group

			group.addTask {
				do {
					var channel: CakeAgentExecuteStream? = nil
					var receivedBytes: Int = 0

					for try await request: Cakeagent_ExecuteRequest in requestStream {
						self.logger.debug("Request: \(request)")

						if case let .size(size) = request.request {

							tty = TTY(tty: size.rows + size.cols > 0)
							tty!.setTermSize(rows: size.rows, cols: size.cols)

						} else if case let .command(command) = request.request {
							guard let tty = tty else {
								throw ServiceError("No TTY")
							}

							// Launch the process
							let cmd: String
							let args: [String]
							let shell: Bool

							switch command.execute {
							case .shell:
								cmd = "/bin/zsh".shell()
								args = ["-i", "-l"]
								shell = true
							case let .command(exec):
								cmd = exec.command
								args = exec.args
								shell = false
							default:
								throw ServiceError("No command")
							}

							process = Process()

							if let process = process {
								process.terminationHandler = { p in
									self.logger.debug("Shell exited")
									grp.cancelAll()
								}

								process.executableURL = URL(fileURLWithPath: cmd.lookupPath())
								process.arguments = args
								process.environment = ProcessInfo.processInfo.environment
								process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
								process.standardInput = tty.fileHandleForReading
								process.standardOutput = tty.fileHandleForWriting
								process.standardError = errorPipe.fileHandleForWriting

								if shell {
									process.standardError = tty.fileHandleForWriting
								} else {
									process.standardError = errorPipe.fileHandleForWriting
								}

								try process.run()
							}

							channel = try await tty.bootstrap(group: self.eventLoop)

							channelContinuation.yield(channel!)
							channelContinuation.finish()

							// Wait NIOAsyncChannel to be ready
							for try await value in outboundStream {
								outputStream = value
								break
							}

						} else if case let .input(datas) = request.request {

							guard let outputStream = outputStream else {
								throw ServiceError("Fatal error no outbound stream")
							}

							receivedBytes += datas.count
							self.logger.info("inbound: \(datas.count), receivedBytes: \(receivedBytes)")

							try await outputStream.write(ByteBuffer(data: datas))
						} else if case .eof = request.request {

							self.logger.info("received EOF")

							if let tty = tty {
								self.logger.info("EOF")
								tty.eof()
							}
							break
						}
					}
				} catch {
					if error is CancellationError == false {
						guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
							self.logger.error("Error reading from shell, \(error)")
							return
						}
					}
				}
			}

			group.addTask {
				do {
					for try await channel in channelStream {
						try await channel.executeThenClose { inbound, outbound in
							var outBytes: Int = 0
							
							outboundContinuation.yield(outbound)
							outboundContinuation.finish()

							for try await reply in inbound {
								outBytes += reply.readableBytes
								self.logger.info("outbound: \(reply.readableBytes), outBytes=\(outBytes)")
								try await responseStream.send(Cakeagent_ExecuteResponse.with { $0.stdout = Data(buffer: reply) })
							}
						}
						break
					}
				} catch {
					if error is CancellationError == false {
						self.logger.error("Error writing to channel, \(error)")
					}
				}
			}

			await group.waitForAll()
		}

		if let process = process {
			if process.isRunning {
				process.waitUntilExit()
			}

			exitCode = process.terminationStatus
		}

		try await responseStream.send(Cakeagent_ExecuteResponse.with { $0.exitCode = exitCode })
	}
}

