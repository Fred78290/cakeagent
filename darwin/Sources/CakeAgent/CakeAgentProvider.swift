import CakeAgentLib
import GRPC
import NIO
import NIOCore
import SwiftProtobuf
import Foundation
import Logging
import Darwin

let cakerSignature = "com.aldunelabs.cakeagent"

class ServiceError : Error, CustomStringConvertible, @unchecked Sendable {
	let description: String
	let exitCode: Int32

	init(_ what: String, _ code: Int32 = -1) {
		self.description = what
		self.exitCode = code
	}
}

extension TaskGroup<Void>: @retroactive @unchecked Sendable {

}

class TTY: @unchecked Sendable {
	let fileHandleForReading: FileHandle
	let fileHandleForWriting: FileHandle

	init() {
		let master = Self.createPTY()	

		self.fileHandleForReading = FileHandle(fileDescriptor: master.0, closeOnDealloc: true)
		self.fileHandleForWriting = FileHandle(fileDescriptor: master.1, closeOnDealloc: true)
	}

	func close() {
		self.fileHandleForReading.closeFile()
		self.fileHandleForWriting.closeFile()
	}

	private static func setupty(_ fd: Int32) -> Int32 {
		let TTY_CTRL_OPTS: tcflag_t = tcflag_t(CS8 | CLOCAL | CREAD)
		let TTY_INPUT_OPTS: tcflag_t = tcflag_t(IGNPAR)
		let TTY_OUTPUT_OPTS:tcflag_t = 0
		let TTY_LOCAL_OPTS:tcflag_t = 0

		var termios_ = termios()

		var res = fcntl(fd, F_GETFL)
		if (res < 0) {
			perror("fcntl F_GETFL error")
			return res
		}

		// set serial nonblocking
		res = fcntl(fd, F_SETFL, res | O_NONBLOCK)
		if (res < 0) {
			perror("fcntl F_SETFL O_NONBLOCK error")
			return res
		}

		// set baudrate to 115200
		tcgetattr(fd, &termios_)
		cfsetispeed(&termios_, speed_t(B115200))
		cfsetospeed(&termios_, speed_t(B115200))
		cfmakeraw(&termios_)

		// This attempts to replicate the behaviour documented for cfmakeraw in
		// the termios(3) manpage.
		termios_.c_iflag = TTY_INPUT_OPTS
		termios_.c_oflag = TTY_OUTPUT_OPTS
		termios_.c_lflag = TTY_LOCAL_OPTS
		termios_.c_cflag = TTY_CTRL_OPTS
		termios_.c_cc.16 = 1 // Darwin.VMIN
		termios_.c_cc.17 = 0 // Darwin.VTIME

		if (tcsetattr(fd, TCSANOW, &termios_) != 0) {
			perror("tcsetattr error")
			return -1
		}
		return 0
	}

	private static func createPTY() -> (Int32, Int32) {
		var tty_fd: Int32 = -1
		var sfd: Int32 = -1
		let tty_path = UnsafeMutablePointer<CChar>.allocate(capacity: 1024)

		defer {
			tty_path.deallocate()
		}

		var res = openpty(&tty_fd, &sfd, tty_path, nil, nil);
		
		if (res < 0) {
			perror("openpty error")
			return (-1, -1)
		}

		res = setupty(tty_fd)
		if (res < 0) {
			return (res, -1)
		}

		res = setupty(sfd)
		if (res < 0) {
			return (res, -1)
		}

		return (tty_fd, sfd)
	}
}


extension FileHandle {
	func makeRaw() -> FileHandle {
		let fd = self.fileDescriptor
		let TTY_CTRL_OPTS: tcflag_t = tcflag_t(CS8 | CLOCAL | CREAD)
		let TTY_INPUT_OPTS: tcflag_t = tcflag_t(IGNPAR)
		let TTY_OUTPUT_OPTS:tcflag_t = 0
		let TTY_LOCAL_OPTS:tcflag_t = 0

		var termios_ = termios()
		var res = fcntl(fd, F_GETFL)
		if res < 0 {
			perror("fcntl F_GETFL error")
			fatalError()
		}

		res = fcntl(fd, F_SETFL, res | O_NONBLOCK)
		if res < 0 {
			perror("fcntl F_SETFL error")
			fatalError()
		}

		tcgetattr(fd, &termios_)
		cfsetispeed(&termios_, speed_t(B115200))
		cfsetospeed(&termios_, speed_t(B115200))
		cfmakeraw(&termios_)

		// This attempts to replicate the behaviour documented for cfmakeraw in
		// the termios(3) manpage.
		termios_.c_iflag = TTY_INPUT_OPTS
		termios_.c_oflag = TTY_OUTPUT_OPTS
		termios_.c_lflag = TTY_LOCAL_OPTS
		termios_.c_cflag = TTY_CTRL_OPTS
		termios_.c_cc.16 = 1 // Darwin.VMIN
		termios_.c_cc.17 = 0 // Darwin.VTIME

		tcsetattr(fd, TCSANOW, &termios_)

		return self
	}
}

final class CakeAgentProvider: Sendable, Cakeagent_AgentAsyncProvider {
	let group: EventLoopGroup
	let logger = Logging.Logger(label: "com.aldunelabs.cakeagent")

	init(group: EventLoopGroup) {
		self.group = group
	}

	func info(request: Google_Protobuf_Empty, context: GRPCAsyncServerCallContext) async throws -> Cakeagent_InfoReply {
		let processInfo = ProcessInfo.processInfo
		var reply = Cakeagent_InfoReply()
		var memory = Cakeagent_InfoReply.MemoryInfo()
		var size: size_t = 0
		var memSize: UInt64 = 0
		var freeMemory: UInt64 = 0

		size = MemoryLayout<UInt64>.size

		sysctlbyname("hw.memsize", &memSize, &size, nil, 0)

		size = MemoryLayout<UInt64>.size
		sysctlbyname("vm.page_free_count", &freeMemory, &size, nil, 0)

		memory.free = freeMemory * UInt64(vm_page_size)
		memory.total = processInfo.physicalMemory
		memory.used = memory.total - memory.free

		reply.cpuCount = Int32(processInfo.processorCount)
		reply.memory = memory

		reply.osname = processInfo.operatingSystemVersionString
		reply.release = "\(processInfo.operatingSystemVersion.majorVersion).\(processInfo.operatingSystemVersion.minorVersion).\(processInfo.operatingSystemVersion.patchVersion)"
		reply.hostname = processInfo.hostName

		var ipAddresses: [String] = []
		var ifaddr: UnsafeMutablePointer<ifaddrs>? = nil

		if getifaddrs(&ifaddr) == 0, let firstAddr = ifaddr {
			var ptr = firstAddr

			while ptr.pointee.ifa_next != nil {
				let interface = ptr.pointee
				let addrFamily = interface.ifa_addr.pointee.sa_family

				if addrFamily == UInt8(AF_INET) || addrFamily == UInt8(AF_INET6) {
					var hostname = [CChar](repeating: 0, count: Int(NI_MAXHOST))

					getnameinfo(interface.ifa_addr,
					            socklen_t(interface.ifa_addr.pointee.sa_len),
					            &hostname, socklen_t(hostname.count),
					            nil, socklen_t(0), NI_NUMERICHOST)

					let address = String(cString: hostname)

					ipAddresses.append(address)
				}

				ptr = interface.ifa_next
			}

			freeifaddrs(ifaddr)
		}

		reply.ipaddresses = ipAddresses

		return reply
	}

	func execute(request: Cakeagent_ExecuteRequest, context: GRPCAsyncServerCallContext) async throws -> Cakeagent_ExecuteReply {
		let process = Process()
		let outputPipe = Pipe()
		let errorPipe = Pipe()
		var outputData = Data()
		var errorData = Data()
		var arguments: [String] = [request.command]

		arguments.append(contentsOf: request.args)

		let outputQueue = DispatchQueue(label: "bash-output-queue")

		logger.info("execute \(request.command)")

		process.executableURL = URL(fileURLWithPath: "/bin/sh")
		process.arguments = ["-c", arguments.joined(separator: " ")]
		process.standardOutput = outputPipe
		process.standardError = errorPipe
		process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser

		if request.hasInput {
			let inputPipe = Pipe()

			process.standardInput = inputPipe

			inputPipe.fileHandleForWriting.writeabilityHandler = { handler in
				handler.write(request.input)
			}	
		}

		outputPipe.fileHandleForReading.readabilityHandler = { handler in
			let data = handler.availableData

			if data.isEmpty == false {
				self.logger.info("outputPipe data \(String(data: data, encoding: .utf8) ?? "")")
				outputQueue.async {
					outputData.append(data)
				}
			}
		}

		errorPipe.fileHandleForReading.readabilityHandler = { handler in
			let data = handler.availableData

			if data.isEmpty == false {
				self.logger.info("errorPipe data \(String(data: data, encoding: .utf8) ?? "")")
				outputQueue.async {
					errorData.append(data)
				}
			}
		}

		try process.run()

		process.waitUntilExit()

		return Cakeagent_ExecuteReply.with { reply in
			if outputData.isEmpty == false {
				reply.output = outputData
			}

			if errorData.isEmpty == false {
				reply.error = errorData
			}

			reply.exitCode = Int32(process.terminationStatus)
		}
	}

	func shell(requestStream: GRPC.GRPCAsyncRequestStream<CakeAgentLib.Cakeagent_ShellMessage>, responseStream: GRPC.GRPCAsyncResponseStreamWriter<CakeAgentLib.Cakeagent_ShellResponse>, context: GRPC.GRPCAsyncServerCallContext) async throws {
		let process = Process()
		let inputPipe = TTY()
		let outputPipe = Pipe()
		let errorPipe = Pipe()

		process.executableURL = URL(fileURLWithPath: "/bin/sh")
		process.arguments = ["-i", "-l"]
		process.standardInput = inputPipe.fileHandleForReading
		process.standardOutput = outputPipe.fileHandleForWriting
		process.standardError = errorPipe.fileHandleForWriting
		process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser

		errorPipe.fileHandleForReading.readabilityHandler = { handler in
			let data = handler.availableData

			if data.isEmpty == false {
				Task {
					_ = try? await responseStream.send(Cakeagent_ShellResponse.with { message in
						message.format = .stderr
						message.datas = data
					})
				}
			}
		}

		let channel: NIOAsyncChannel<ByteBuffer, ByteBuffer> = try await NIOPipeBootstrap(group: self.group)
			.takingOwnershipOfDescriptors(input: outputPipe.fileHandleForReading.fileDescriptor, output: inputPipe.fileHandleForWriting.fileDescriptor) { channel in
				return channel.eventLoop.makeCompletedFuture {
					try NIOAsyncChannel(wrappingChannelSynchronously: channel, configuration: .init(isOutboundHalfClosureEnabled: true))
				}
			}

		do {
			try process.run()

			defer {
				process.terminate()
			}

			try await channel.executeThenClose { inbound, outbound in
				await withTaskGroup(of: Void.self) { group in
					let grp = group

					process.terminationHandler = { p in
						self.logger.debug("Shell exited")
						grp.cancelAll()
					}

					group.addTask {
						do {
							for try await message: Cakeagent_ShellMessage in requestStream {
								try await outbound.write(ByteBuffer(data: message.datas))
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
							for try await reply in inbound {
								try await responseStream.send(Cakeagent_ShellResponse.with { $0.datas = Data(buffer: reply) })
							}
						} catch {
							if error is CancellationError == false {
								self.logger.error("Error writing to channel, \(error)")
							}
						}
					}

					await group.waitForAll()

					process.terminationHandler = nil
				}
			}

			try await responseStream.send(Cakeagent_ShellResponse.with { $0.format = .end })

			self.logger.debug("Exit shell")
		} catch {
			if error is CancellationError == false {
				self.logger.error("Shell stopped on error, \(error)")
			}
		}
	}
}	
