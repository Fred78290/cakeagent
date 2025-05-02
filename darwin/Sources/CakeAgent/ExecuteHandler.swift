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

final class OutputAsyncStream: Sendable {
	let name: String
	let (stream, continuation) = AsyncStream.makeStream(of: Cakeagent_ExecuteResponse.self)

	init(name: String) {
		self.name = name
	}
}


public class ExecuteHandleStream {
	let requestStream: Cakeagent_ExecuteRequestStream
	let responseStream: Cakeagent_ExecuteResponseStream
	let eventLoop: EventLoop

	private struct OuputStream {
		let fileHandle: FileHandle
		let fd: Int32
		let name: String
		let channel: Int32
		var outBytes: Int = 0

		init(fileHandle: FileHandle, channel: Int32, name: String) {
			self.fileHandle = fileHandle
			self.name = name
			self.channel = channel
			self.fd = fileHandle.fileDescriptor
		}
	}

	init(on: EventLoop, requestStream: Cakeagent_ExecuteRequestStream, responseStream: Cakeagent_ExecuteResponseStream) {
		self.eventLoop = on
		self.requestStream = requestStream
		self.responseStream = responseStream
	}

	private func createTTY(size: Cakeagent_TerminalSize) throws -> TTY {
		let tty = try TTY(tty: size.rows + size.cols > 0)

		try tty.setTermSize(rows: size.rows, cols: size.cols)

		return tty
	}

	private func createProcess(command: Cakeagent_ExecuteCommand, tty: TTY) async throws -> Process {
		let process: Process = Process()
		let cmd: String
		let args: [String]
		//let shell: Bool

		switch command.execute {
		case .shell:
			cmd = "/bin/zsh".shell()
			args = ["-i", "-l"]
		//shell = true
		case let .command(exec):
			cmd = exec.command
			args = exec.args
		//shell = false
		default:
			throw ServiceError("No command")
		}

		process.executableURL = URL(fileURLWithPath: cmd.lookupPath())
		process.arguments = args
		process.environment = ProcessInfo.processInfo.environment
		process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
		process.standardInput = tty.fileHandleForStdinReading
		process.standardOutput = tty.fileHandleForStdoutWriting
		process.standardError = tty.fileHandleForStderrWriting

		process.terminationHandler = { p in
			if Logger.Level() >= .debug {
				Logger(self).debug("Process exited")
			}
		}

		try process.run()

		if Logger.Level() >= .debug {
			Logger(self).debug("Shell started")
		}

		return process
	}

	private func forwardOutput(output: OuputStream, process: Process, logger: Logger) async -> OuputStream {
		var output = output
		let bufSize = 4096
		let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufSize)
		let logLevel = Logger.Level()

		logger.debug("Enter \(output.name)")

		defer {
			buffer.deallocate()
			logger.debug("Leave \(output.name)")
		}

		do {
			while true {
				var timeout = timeval(tv_sec: 0, tv_usec: 100)
				var readfds = fd_set()

				__darwin_fd_set(output.fd, &readfds)

				if select(output.fd+1, &readfds, nil, nil, &timeout) < 0 {
					if errno == EINTR {
						continue
					}
					logger.error("Select \(output.name) failed, \(String(cString: strerror(errno)))")
					break
				}

				if __darwin_fd_isset(output.fd, &readfds) == 0 {
					if process.isRunning == false {
						if logLevel >= .debug {
							logger.debug("EOF \(output.name), outBytes=\(output.outBytes)")
						}
						break
					}
					continue
				}

				let bytesRead = read(output.fd, buffer, bufSize)

				if bytesRead > 0 {
					let data: Data = Data(bytes: buffer, count: bytesRead)

					output.outBytes += bytesRead

					if logLevel >= .trace {
						logger.trace("\(output.name): bytesRead: \(bytesRead), outBytes: \(output.outBytes) [\(String(data: data, encoding: .utf8) ?? "<unknown>")]")
					} else if logLevel >= .debug {
						logger.debug("\(output.name): bytesRead: \(bytesRead), outBytes: \(output.outBytes)")
					}

					_ = try await self.responseStream.send(Cakeagent_ExecuteResponse.with {
						if output.channel == STDOUT_FILENO {
							$0.stdout = data
						} else {
							$0.stderr = data
						}
					})
				} else if bytesRead < 0 {
					if errno == EBADF || errno == EAGAIN {
						if process.isRunning == false {
							if logLevel >= .debug {
								logger.debug("EOF \(output.name), outBytes=\(output.outBytes)")
							}
							break
						} else if logLevel >= .debug {
							logger.debug("EAGAIN \(output.name), outBytes=\(output.outBytes)")
						}
					} else {
						logger.error("Error reading from \(output.name): \(String(cString: strerror(errno)))")
						break
					}
				} else if process.isRunning == false {
					if logLevel >= .debug {
						logger.debug("EOF reading output, \(output.name)  outBytes=\(output.outBytes), \(String(cString: strerror(errno)))")
					}
					break
				}
			}
		} catch {
			logger.error(error)
		}

		return output
	}

	func stream() async throws {
		var exitCode: Int32 = -1
		let process: Process
		let tty: TTY
		var iterator = requestStream.makeAsyncIterator()
		let logger = Logger(self)

		// Read terminal size
		if let request = try await iterator.next() {
			if case let .size(size) = request.request {
				tty = try self.createTTY(size: size)
			} else {
				throw ServiceError("empty terminal size")
			}
		} else {
			throw ServiceError("Failed to receive terminal size message")
		}

		// Read request command
		if let request = try await iterator.next() {
			if case let .command(command) = request.request {
				process = try await self.createProcess(command: command, tty: tty)
			} else {
				throw ServiceError("empty command")
			}
		} else {
			throw ServiceError("Failed to receive command message")
		}

		defer {
			logger.debug("leave stream")
			tty.close()
		}

		_ = try await self.responseStream.send(Cakeagent_ExecuteResponse.with {
			$0.established = true
		})

		let inputStreamTask = Task {
			logger.debug("enter inboundStream")

			defer {
				logger.debug("leave inboundStream")
			}

			// Handle all messages
			do {
				while let request = try await iterator.next(), process.isRunning {
					if case let .input(datas) = request.request {
						tty.writeToStdin(datas)
					} else if case let .size(size) = request.request {
						try tty.setTermSize(rows: size.rows, cols: size.cols)
					} else if case .eof = request.request {
						tty.eof()
						break
					}
				}
			} catch {
				if error is CancellationError == false {
					guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
						logger.error("Error reading from shell, \(error)")
						return
					}
				}
			}
		}

		await withTaskGroup{ group in
			let outputs = [
				OuputStream(fileHandle: tty.fileHandleForStdoutReading, channel: STDOUT_FILENO, name: "stdout"),
				OuputStream(fileHandle: tty.fileHandleForStderrReading, channel: STDERR_FILENO, name: "stderr")
			]

			outputs.forEach { (output: OuputStream) in
				group.addTask {
					await self.forwardOutput(output: output, process: process, logger: logger)
				}
			}

			await group.waitForAll()

			inputStreamTask.cancel()
		}

		if process.isRunning {
			logger.debug("process.waitUntilExit")
			process.waitUntilExit()
		}

		exitCode = process.terminationStatus

		logger.debug("exitCode: \(exitCode)")

		try await responseStream.send(Cakeagent_ExecuteResponse.with { $0.exitCode = exitCode })
	}
}

