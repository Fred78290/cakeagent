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

extension Cakeagent_ShellRequest {
	var isCommand: Bool {
		guard case .command = self.request else {
			return false
		}
		
		return true
	}
	
	var isInput: Bool {
		guard case .input = self.request else {
			return false
		}
		
		return true
	}
}

class TTY: @unchecked Sendable {
	let ptx: FileHandle
	let pty: FileHandle

	init() {
		let master = Self.createPTY()	

		self.ptx = FileHandle(fileDescriptor: master.0, closeOnDealloc: true)
		self.pty = FileHandle(fileDescriptor: master.1, closeOnDealloc: true)
	}

	func close() {
		self.ptx.closeFile()
		self.pty.closeFile()
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

		let res = openpty(&tty_fd, &sfd, tty_path, nil, nil);

		if (res < 0) {
			perror("openpty error")
			return (-1, -1)
		}

/*		res = setupty(tty_fd)
		if (res < 0) {
			return (res, -1)
		}

		res = setupty(sfd)
		if (res < 0) {
			return (res, -1)
		}
*/
		return (tty_fd, sfd)
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

	func execute(command: Cakeagent_ExecuteCommand, requestStream: GRPCAsyncRequestStream<Cakeagent_ShellRequest>, responseStream: GRPCAsyncResponseStreamWriter<Cakeagent_ShellResponse>, context: GRPCAsyncServerCallContext) async throws {
		let process = Process()
		let tty = TTY()
		let errorPipe = Pipe()

		errorPipe.fileHandleForReading.readabilityHandler = { handler in
			let data = handler.availableData

			if data.isEmpty == false {
				Task {
					let message = Cakeagent_ShellResponse.with {
						$0.stderr = data
					}

					_ = try? await responseStream.send(message)
				}
			}
		}

		process.executableURL = URL(fileURLWithPath: command.command)
		process.arguments = command.args
		process.standardInput = tty.pty
		process.standardOutput = tty.pty
		process.standardError = errorPipe.fileHandleForWriting
		process.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser

		let channel: NIOAsyncChannel<ByteBuffer, ByteBuffer> = try await NIOPipeBootstrap(group: self.group)
			.takingOwnershipOfDescriptor(inputOutput: tty.ptx.fileDescriptor) { channel in
				return channel.eventLoop.makeCompletedFuture {
					try NIOAsyncChannel(wrappingChannelSynchronously: channel, configuration: .init(isOutboundHalfClosureEnabled: true))
				}
			}

		do {
			defer {
				process.terminate()
			}

			try await channel.executeThenClose { inbound, outbound in
				try process.run()

				await withTaskGroup(of: Void.self) { group in
					let grp = group

					process.terminationHandler = { p in
						self.logger.debug("Shell exited")
						grp.cancelAll()
					}

					group.addTask {
						do {
							for try await request: Cakeagent_ShellRequest in requestStream {
								if case let .input(message) = request.request {
									tty.pty.setTermSize(rows: message.rows, cols: message.cols)
									if message.datas.isEmpty == false {
										try await outbound.write(ByteBuffer(data: message.datas))
									}
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
							for try await reply in inbound {
								try await responseStream.send(Cakeagent_ShellResponse.with { $0.stdout = Data(buffer: reply) })
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

			try await responseStream.send(Cakeagent_ShellResponse.with { $0.exitCode = 0 })

			self.logger.debug("Exit shell")
		} catch {
			if error is CancellationError == false {
				try? await responseStream.send(Cakeagent_ShellResponse.with { $0.stderr = error.localizedDescription.data(using: .utf8)! })
				try? await responseStream.send(Cakeagent_ShellResponse.with { $0.exitCode = 1 })

				self.logger.error("Shell stopped on error, \(error)")
			}
		}
	}

	func execute(requestStream: GRPCAsyncRequestStream<Cakeagent_ShellRequest>, responseStream: GRPCAsyncResponseStreamWriter<Cakeagent_ShellResponse>, context: GRPCAsyncServerCallContext) async throws {
		guard let request = try await requestStream.first(where: {  $0.isCommand }) else {
			throw ServiceError("The firt message must be command")
		}
			
		try await self.execute(command: request.command, requestStream: requestStream, responseStream: responseStream, context: context)
	}
	
	func shell(requestStream: GRPCAsyncRequestStream<Cakeagent_ShellRequest>, responseStream: GRPCAsyncResponseStreamWriter<Cakeagent_ShellResponse>, context: GRPCAsyncServerCallContext) async throws {
		let command = Cakeagent_ExecuteCommand.with {
			$0.cols = 80
			$0.rows = 24
			$0.command = "/bin/sh"
			$0.args = ["-i", "-l"]
		}

		try await self.execute(command: command, requestStream: requestStream, responseStream: responseStream, context: context)
	}

	func mount(request: Cakeagent_MountRequest, context: GRPC.GRPCAsyncServerCallContext) async throws -> Cakeagent_MountReply {
		Cakeagent_MountReply.with { $0.error = "Not supported" }
	}

	func umount(request: Cakeagent_MountRequest, context: GRPC.GRPCAsyncServerCallContext) async throws -> Cakeagent_MountReply {
		Cakeagent_MountReply.with { $0.error = "Not supported" }
	}

}	
