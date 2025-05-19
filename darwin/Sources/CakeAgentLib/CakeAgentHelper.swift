import ArgumentParser
@preconcurrency import GRPC
import Foundation
import NIO
import NIOPosix
import NIOSSL
import Semaphore
import NIOPortForwarding


typealias CakeAgentExecuteStream = BidirectionalStreamingCall<CakeAgent.ExecuteRequest, CakeAgent.ExecuteResponse>

public protocol CakeAgentClientInterceptorState {
	func restoreState()
}

#if TRACE
	func redbold(_ string: String) {
		FileHandle.standardError.write("\u{001B}[0;31m\u{001B}[1m\(string)\u{001B}[0m\n".data(using: .utf8)!)
	}
#endif

extension CakeAgentExecuteStream {
	@discardableResult
	func sendTerminalSize(rows: Int32, cols: Int32) -> EventLoopFuture<Void> {
		let message = CakeAgent.ExecuteRequest.with {
			$0.size = CakeAgent.ExecuteRequest.TerminalSize.with {
				$0.rows = rows
				$0.cols = cols
			}
		}

		return self.sendMessage(message)
	}

	@discardableResult
	func sendCommand(command: String, arguments: [String]) -> EventLoopFuture<Void> {
		let message = CakeAgent.ExecuteRequest.with {
			$0.command = CakeAgent.ExecuteRequest.ExecuteCommand.with {
				$0.command = CakeAgent.ExecuteRequest.ExecuteCommand.Command.with {
					$0.command = command
					$0.args = arguments
				}
			}
		}

		return self.sendMessage(message)
	}

	@discardableResult
	func sendShell() -> EventLoopFuture<Void> {
		let message = CakeAgent.ExecuteRequest.with {
			$0.command = CakeAgent.ExecuteRequest.ExecuteCommand.with {
				$0.shell = true
			}
		}

		return self.sendMessage(message)
	}

	@discardableResult
	func sendBuffer(_ buffer: ByteBuffer) -> EventLoopFuture<Void> {
		let message = CakeAgent.ExecuteRequest.with {
			$0.input = Data(buffer: buffer)
		}

		return self.sendMessage(message)
	}

	@discardableResult
	func sendEof() -> EventLoopFuture<Void> {
		let message = CakeAgent.ExecuteRequest.with {
			$0.eof = true
		}

		return self.sendMessage(message)
	}

	@discardableResult
	func end() -> EventLoopFuture<Void> {
		#if TRACE
			redbold("Send end")
		#endif
		return self.sendEnd()
	}
}

extension CakeAgentClient {
	public func close() async throws {
		try await self.channel.close().get()
	}

	public func close() -> EventLoopFuture<Void> {
		self.channel.close()
	}

	public func close(promise: EventLoopPromise<Void>) {
		self.channel.close(promise: promise)
	}

	public func closeSync() throws {
		try self.channel.close().wait()
	}
}

public enum Status: String, Sendable, Codable {
	case running
	case stopped
	case unknown
}

public struct ShutdownReply: Sendable, Codable {
	public let exitCode: Int32
	public let stdout: String?
	public let stderr: String?

	public init() {
		self.exitCode = 0
		self.stdout = nil
		self.stderr = nil
	}

	public init(exitCode: Int32, stdout: String? = nil, stderr: String? = nil) {
		self.stdout = stdout
		self.stderr = stderr
		self.exitCode = exitCode
	}
}

public enum ResizeReply: Sendable, Codable {
	case success(Bool)
	case failure(String)
}

public struct DiskInfo: Sendable, Codable {
	public var device: String
	public var mount: String
	public var fsType: String
	public var total: UInt64
	public var free: UInt64
	public var used: UInt64

	init() {
		self.device = ""
		self.mount = ""
		self.fsType = ""
		self.total = 0
		self.free = 0
		self.used = 0
	}

	public init(device: String, mount: String, fsType: String, total: UInt64, free: UInt64, used: UInt64) {
		self.device = device
		self.mount = mount
		self.fsType = fsType
		self.total = total
		self.free = free
		self.used = used
	}
}

public struct AttachedNetwork: Sendable, Codable {
	public var network: String
	public var mode: String?
	public var macAddress: String?

	public init() {
		self.network = ""
		self.mode = nil
		self.macAddress = nil
	}

	public init(network: String, mode: String?, macAddress: String?) {
		self.network = network
		self.mode = mode
		self.macAddress = macAddress
	}
}

public struct TunnelInfo: Sendable, Codable {
	public struct UnixDomainSocket: Sendable, Codable {
		public var proto: MappedPort.Proto
		public var host: String
		public var guest: String

		public init() {
			self.proto = .tcp
			self.host = ""
			self.guest = ""
		}

		public init(proto: MappedPort.Proto, host: String, guest: String) {
			self.proto = proto
			self.host = host
			self.guest = guest
		}
	}

	public enum OneOf: Sendable, Codable {
		case forward(ForwardedPort)
		case unixDomain(UnixDomainSocket)
	}

	public var oneOf: OneOf

	public init(from: ForwardedPort) {
		self.oneOf = .forward(from)
	}

	public init(from: UnixDomainSocket) {
		self.oneOf = .unixDomain(from)
	}

	public init(from decoder: Decoder) throws {
		let container: KeyedDecodingContainer<Self.CodingKeys> = try decoder.container(keyedBy: CodingKeys.self)

		if let forward = try container.decodeIfPresent(ForwardedPort.self, forKey: .forward) {
			self.oneOf = .forward(forward)
		} else if let unixDomainSocket = try container.decodeIfPresent(UnixDomainSocket.self, forKey: .unixDomain) {
			self.oneOf = .unixDomain(unixDomainSocket)
		} else {
			throw NSError(domain: "CakeAgent", code: 0, userInfo: [NSLocalizedDescriptionKey: "Invalid TunnelInfo"])
		}
	}

	public func encode(to encoder: Encoder) throws {
		var container: KeyedEncodingContainer<CodingKeys> = encoder.container(keyedBy: CodingKeys.self)

		switch oneOf {
		case .forward(let value):
			try container.encode(value, forKey: .forward)
		case .unixDomain(let value):
			try container.encode(value, forKey: .unixDomain)
		}
	}

	enum CodingKeys: String, CodingKey {
		case forward
		case unixDomain
	}
}

public struct SocketInfo: Sendable, Codable {
	public enum Mode: Sendable, Codable {
		case bind // = 0
		case connect // = 1
		case tcp // = 2
		case udp // = 3

		var rawValue: Int {
			switch self {
			case .bind:
				return 0
			case .connect:
				return 1
			case .tcp:
				return 2
			case .udp:
				return 3
			}
		}

		public init?(rawValue: Int) {
			switch rawValue {
			case 0:
				self = .bind
			case 1:
				self = .connect
			case 2:
				self = .tcp
			case 3:
				self = .udp
			default:
				return nil
			}
		}
	}

	public var mode: Mode
	public var host: String
	public var port: Int32

	public init() {
		self.mode = .bind
		self.host = ""
		self.port = 0
	}

	public init(mode: Mode, host: String, port: Int32) {
		self.mode = mode
		self.host = host
		self.port = port
	}
}

public struct InfoReply: Sendable, Codable {
	public var name: String
	public var version: String?
	public var uptime: UInt64?
	public var memory: MemoryInfo?
	public var cpuCount: Int32
	public var diskInfos: [DiskInfo]
	public var ipaddresses: [String]
	public var osname: String
	public var hostname: String?
	public var release: String?
	public var mounts: [String]?
	public var status: Status
	public var attachedNetworks: [AttachedNetwork]?
	public var tunnelInfos: [TunnelInfo]?
	public var socketInfos: [SocketInfo]?

	init() {
		self.name = ""
		self.version = nil
		self.uptime = 0
		self.memory = nil
		self.cpuCount = 0
		self.diskInfos = []
		self.ipaddresses = []
		self.osname = ""
		self.hostname = nil
		self.release = nil
		self.status = .stopped
		self.mounts = nil
		self.attachedNetworks = nil
		self.tunnelInfos = nil
		self.socketInfos = nil
	}

	public init(info: CakeAgent.InfoReply) {
		self.name = info.hostname
		self.version = info.version
		self.uptime = info.uptime
		self.cpuCount = info.cpuCount
		self.ipaddresses = info.ipaddresses
		self.osname = info.osname
		self.hostname = info.hostname
		self.release = info.release
		self.status = .running
		self.diskInfos = info.diskInfos.map { diskInfo in
			DiskInfo(device: diskInfo.device, mount: diskInfo.mount, fsType: diskInfo.fsType, total: diskInfo.size, free: diskInfo.free, used: diskInfo.used)
		}

		if info.hasMemory {
			self.memory = MemoryInfo.with {
				$0.total = info.memory.total
				$0.free = info.memory.free
				$0.used = info.memory.used
			}
		}
	}

	public struct MemoryInfo: Sendable, Codable {
		public var total: UInt64?
		public var free: UInt64?
		public var used: UInt64?

		init() {
			self.total = nil
			self.free = nil
			self.used = nil
		}

		public static func with(
			_ populator: (inout Self) throws -> Void
		) rethrows -> Self {
			var message = Self()
			try populator(&message)
			return message
		}
	}

	public func toJSON() -> String {
		let encoder = JSONEncoder()
		encoder.outputFormatting = [.prettyPrinted, .withoutEscapingSlashes]

		return String(data: try! encoder.encode(self), encoding: .utf8)!
	}

	public static func with(
		_ populator: (inout Self) throws -> Void
	) rethrows -> Self {
		var message = Self()
		try populator(&message)
		return message
	}
}

final class CakeChannelStreamer: @unchecked Sendable {
	let eventLoop: EventLoop
	let inputHandle: FileHandle
	let outputHandle: FileHandle
	let errorHandle: FileHandle
	let semaphore = AsyncSemaphore(value: 0)
	let isTTY: Bool
	var pipeChannel: NIOAsyncChannel<ByteBuffer, ByteBuffer>? = nil
	var exitCode: Int32 = 0
	var receivedLength: UInt64 = 0
	var term: termios? = nil

	enum ExecuteCommand: Equatable, Sendable {
		case execute(String, [String])
		case shell(Bool = true)
	}

	init(on: EventLoop, inputHandle: FileHandle, outputHandle: FileHandle, errorHandle: FileHandle) {
		self.eventLoop = on
		self.inputHandle = inputHandle
		self.outputHandle = outputHandle
		self.errorHandle = errorHandle
		self.isTTY = inputHandle.isTTY() && outputHandle.isTTY()
	}

	func handleResponse(response: CakeAgent.ExecuteResponse) -> Void {
		guard let pipeChannel = self.pipeChannel else {
			return
		}

		do {
			if case let .exitCode(code) = response.response {
				#if TRACE
					redbold("exitCode=\(code)")
				#endif
				self.exitCode = code
				_ = pipeChannel.channel.close()
				self.semaphore.signal()
			} else if case let .stdout(datas) = response.response {
				self.receivedLength += UInt64(datas.count)
				#if TRACE
					redbold("message length: \(datas.count), receivedLength=\(self.receivedLength)")
				#else
					try self.outputHandle.write(contentsOf: datas)
				#endif
			} else if case let .stderr(datas) = response.response {
				try self.errorHandle.write(contentsOf: datas)
			} else if case .established = response.response {
				if self.inputHandle.isTTY() {
					self.term = try self.inputHandle.makeRaw()
				}
			}
		} catch {
			if error is CancellationError == false {
				guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
					let errMessage = "error: \(error)\n".data(using: .utf8)!

					if FileHandle.standardError.fileDescriptor != self.errorHandle.fileDescriptor {
						FileHandle.standardError.write(errMessage)
					}

					self.errorHandle.write(errMessage)
					return
				}
			}
		}
	}

	@discardableResult
	func setTerminalSize(stream: CakeAgentExecuteStream) -> EventLoopFuture<Void> {
		let size = self.isTTY ? self.outputHandle.getTermSize() : (rows: 0, cols: 0)

		return stream.sendTerminalSize(rows: size.rows, cols: size.cols)
	}

	func stream(command: ExecuteCommand, handler: @escaping () -> CakeAgentExecuteStream) async throws -> Int32 {
		let stream: CakeAgentExecuteStream = handler()
		let sigwinch: DispatchSourceSignal?

		defer {
			if var term = self.term {
				try? inputHandle.restoreState(&term)
			}
		}

		if self.isTTY {
			let sig = DispatchSource.makeSignalSource(signal: SIGWINCH)

			sigwinch = sig

			sig.setEventHandler {
				stream.eventLoop.execute {
					self.setTerminalSize(stream: stream)
				}
			}

			sig.activate()
		} else {
			sigwinch = nil
		}

		self.setTerminalSize(stream: stream)

		let fd: CInt
		let fileProxy: Pipe?
		let fileSize: UInt64

		if try self.inputHandle.fileDescriptorIsFile() {
			let proxy = Pipe()
			let currentOffset = try self.inputHandle.offset()

			fd = proxy.fileHandleForReading.fileDescriptor
			fileProxy = proxy
			fileSize = try self.inputHandle.seekToEnd()

			try self.inputHandle.seek(toOffset: currentOffset)
		} else {
			fd = dup(self.inputHandle.fileDescriptor)
			fileProxy = nil
			fileSize = 0
		}

		defer {
			#if TRACE
				redbold("Exit receivedLength=\(self.receivedLength)")
			#endif
			if let sig = sigwinch {
				sig.cancel()
			}
			stream.end()

			if let fileProxy = fileProxy {
				fileProxy.fileHandleForWriting.closeFile()
				fileProxy.fileHandleForReading.closeFile()
			}
		}

		let channel = try await stream.subchannel.flatMapThrowing { streamChannel in
			return Task {
				return try await NIOPipeBootstrap(group: self.eventLoop)
					.channelOption(.autoRead, value: true)
					.takingOwnershipOfDescriptor(input: fd) { pipeChannel in
						if let proxy = fileProxy {
							proxy.fileHandleForWriting.writeabilityHandler = { handle in
								if let data = try? self.inputHandle.read(upToCount: 1024) {
									if data.isEmpty == false {
										handle.write(data)
									}
								}
							}
						}

						return pipeChannel.eventLoop.makeCompletedFuture {
							try NIOAsyncChannel<ByteBuffer, ByteBuffer>(wrappingChannelSynchronously: pipeChannel)
						}
					}
			}
		}.get().value

		self.pipeChannel = channel

		try await pipeChannel!.executeThenClose { inbound, outbound in
			stream.status.whenComplete { result in
				switch result {
				case .failure(let err):
					#if TRACE
						redbold("Tunnel error: \(err)")
					#endif
					channel.channel.close(promise: nil)
				case .success(let status):
					#if TRACE
						redbold("Tunnel status: \(status)")
					#endif
					if status.code != .ok {
						channel.channel.close(promise: nil)
					}
				}
			}

			if case let .execute(cmd, arguments) = command {
				stream.sendCommand(command: cmd, arguments: arguments)
			} else if case .shell = command {
				stream.sendShell()
			}

			do {
				var bufLength = fileSize

				#if TRACE
					redbold("Input size \(fileSize)")
				#endif

				for try await buffer: ByteBuffer in inbound {
					stream.sendBuffer(buffer)

					#if TRACE
						redbold("Read=\(buffer.readableBytes)")
					#endif

					if fileSize > 0 {
						bufLength -= UInt64(buffer.readableBytes)
						#if TRACE
							redbold("Remains bufLength=\(bufLength), receivedLength=\(self.receivedLength)")
						#endif
						if bufLength <= 0 {
							break
						}
					} else {
						bufLength += UInt64(buffer.readableBytes)
					}
				}

				stream.sendEof()

				#if TRACE
					redbold("EOF bufLength=\(bufLength), receivedLength=\(self.receivedLength)")
				#endif
			} catch {
				#if TRACE
					redbold("error: \(error)")
				#endif
				if error is CancellationError == false {
					guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
						let errMessage = "error: \(error)\n".data(using: .utf8)!

						if FileHandle.standardError.fileDescriptor != self.errorHandle.fileDescriptor {
							FileHandle.standardError.write(errMessage)
						}

						errorHandle.write(errMessage)
						return
					}
				}
			}
		}

		await self.semaphore.wait()

		return self.exitCode
	}
}

public struct CakeAgentHelper: Sendable {
	internal let eventLoopGroup: EventLoopGroup
	internal let client: CakeAgentClient

	public struct RunReply: Sendable {
		public var exitCode: Int32
		public var stdout: Data
		public var stderr: Data
	}

	public init(on: EventLoopGroup, client: CakeAgentClient) {
		self.eventLoopGroup = on
		self.client = client
	}

	public init(on: EventLoopGroup,
	            listeningAddress: URL,
	            connectionTimeout: Int64,
	            caCert: String?,
	            tlsCert: String?,
	            tlsKey: String?,
	            retries: ConnectionBackoff.Retries = .unlimited,
	            interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? = nil) throws {
		self.eventLoopGroup = on
		self.client = try Self.createClient(on: on,
		                                    listeningAddress: listeningAddress,
		                                    connectionTimeout: connectionTimeout,
		                                    caCert: caCert,
		                                    tlsCert: tlsCert,
		                                    tlsKey: tlsKey,
		                                    retries: retries,
		                                    interceptors: interceptors)
	}

	public static func createClient(on: EventLoopGroup,
	                                listeningAddress: URL,
	                                connectionTimeout: Int64,
	                                caCert: String?,
	                                tlsCert: String?,
	                                tlsKey: String?,
	                                retries: ConnectionBackoff.Retries = .unlimited,
	                                interceptors: CakeAgentServiceClientInterceptorFactoryProtocol? = nil) throws -> CakeAgentClient {
		let target: ConnectionTarget

		if listeningAddress.scheme == "unix" || listeningAddress.isFileURL {
			target = ConnectionTarget.unixDomainSocket(listeningAddress.path())
		} else if listeningAddress.scheme == "tcp" {
			target = ConnectionTarget.hostAndPort(listeningAddress.host ?? "127.0.0.1", listeningAddress.port ?? 5000)
		} else {
			throw ValidationError("unsupported address scheme: \(listeningAddress)")
		}

		var clientConfiguration = ClientConnection.Configuration.default(target: target, eventLoopGroup: on)

		if let tlsCert = tlsCert, let tlsKey = tlsKey {
			let tlsCert = try NIOSSLCertificate(file: tlsCert, format: .pem)
			let tlsKey = try NIOSSLPrivateKey(file: tlsKey, format: .pem)
			let trustRoots: NIOSSLTrustRoots

			if let caCert: String = caCert {
				trustRoots = .certificates([try NIOSSLCertificate(file: caCert, format: .pem)])
			} else {
				trustRoots = NIOSSLTrustRoots.default
			}

			clientConfiguration.tlsConfiguration = GRPCTLSConfiguration.makeClientConfigurationBackedByNIOSSL(
				certificateChain: [.certificate(tlsCert)],
				privateKey: .privateKey(tlsKey),
				trustRoots: trustRoots,
				certificateVerification: .noHostnameVerification)
		}

		if retries != .unlimited {
			clientConfiguration.connectionBackoff = ConnectionBackoff(maximumBackoff: TimeInterval(connectionTimeout), minimumConnectionTimeout: TimeInterval(connectionTimeout), retries: retries)
		} else {
			clientConfiguration.connectionBackoff = ConnectionBackoff(maximumBackoff: TimeInterval(connectionTimeout))
		}

		return CakeAgentClient(channel: ClientConnection(configuration: clientConfiguration), interceptors: interceptors)
	}

	public func resizeDisk(callOptions: CallOptions? = nil) throws -> ResizeReply {
		let response = client.resizeDisk(.init(), callOptions: callOptions)
		let infos = try response.response.wait()

		if case let .success(success) = infos.response {
			return .success(success)
		} else if case let .failure(error) = infos.response {
			return .failure(error)
		}

		return .failure("unknown error")
	}

	public func info(callOptions: CallOptions? = nil) throws -> InfoReply {
		let response = client.info(.init(), callOptions: callOptions)

		return InfoReply(info: try response.response.wait())
	}

	public func run(command: String,
	                arguments: [String],
	                input: Data? = nil,
	                callOptions: CallOptions? = nil) throws -> RunReply {

		let response = try client.run(CakeAgent.RunCommand.with { req in
			if let input = input {
				req.input = input
			}

			req.command = CakeAgent.RunCommand.Command.with { 
				$0.command = command
				$0.args = arguments
			}
		}).response.wait()

		return RunReply(exitCode: response.exitCode, stdout: response.stdout, stderr: response.stderr)
	}

	public func shutdown(callOptions: CallOptions? = nil) throws -> ShutdownReply {
		let response = try client.shutdown(.init()).response.wait()

		return ShutdownReply(exitCode: response.exitCode, stdout: String(data: response.stdout, encoding: .utf8), stderr: String(data: response.stderr, encoding: .utf8))
	}

	public func run(command: String,
	                arguments: [String],
	                inputHandle: FileHandle = FileHandle.standardInput,
	                outputHandle: FileHandle = FileHandle.standardOutput,
	                errorHandle: FileHandle = FileHandle.standardError,
	                callOptions: CallOptions? = nil) throws -> Int32 {
		let response = try client.run(CakeAgent.RunCommand.with { req in
			if isatty(inputHandle.fileDescriptor) == 0 {
				req.input = inputHandle.readDataToEndOfFile()
			}

			req.command = CakeAgent.RunCommand.Command.with { 
				$0.command = command
				$0.args = arguments
			}
		}).response.wait()

		if response.stderr.isEmpty == false {
			errorHandle.write(response.stderr)
		}

		if response.stdout.isEmpty == false {
			outputHandle.write(response.stdout)
		}

		return response.exitCode
	}

	func exec(command: CakeChannelStreamer.ExecuteCommand,
	          inputHandle: FileHandle = FileHandle.standardInput,
	          outputHandle: FileHandle = FileHandle.standardOutput,
	          errorHandle: FileHandle = FileHandle.standardError,
	          callOptions: CallOptions? = nil) async throws -> Int32 {
		let handler = CakeChannelStreamer(on: self.eventLoopGroup.next(), inputHandle: inputHandle, outputHandle: outputHandle, errorHandle: errorHandle)
		defer {
			if let factory = client.interceptors as? CakeAgentClientInterceptorState {
				factory.restoreState()
			}
		}

		return try await handler.stream(command: command) {
			return client.execute(callOptions: callOptions, handler: handler.handleResponse)
		}
	}

	public func exec(command: String,
	                 arguments: [String],
	                 inputHandle: FileHandle = FileHandle.standardInput,
	                 outputHandle: FileHandle = FileHandle.standardOutput,
	                 errorHandle: FileHandle = FileHandle.standardError,
	                 callOptions: CallOptions? = nil) async throws -> Int32 {
		return try await self.exec(command: .execute(command, arguments), inputHandle: inputHandle, outputHandle: outputHandle, errorHandle: errorHandle, callOptions: callOptions)
	}

	public func shell(inputHandle: FileHandle = FileHandle.standardInput,
	                  outputHandle: FileHandle = FileHandle.standardOutput,
	                  errorHandle: FileHandle = FileHandle.standardError,
	                  callOptions: CallOptions? = nil) async throws -> Int32 {
		return try await self.exec(command: .shell(), inputHandle: inputHandle, outputHandle: outputHandle, errorHandle: errorHandle, callOptions: callOptions)
	}

	public func mount(request: CakeAgent.MountRequest, callOptions: CallOptions? = nil) throws -> CakeAgent.MountReply {
		let response = client.mount(request, callOptions: callOptions)

		return try response.response.wait()
	}

	public func umount(request: CakeAgent.MountRequest, callOptions: CallOptions? = nil) throws -> CakeAgent.MountReply {
		let response = client.umount(request, callOptions: callOptions)

		return try response.response.wait()
	}

	public func tunnel(bindAddress: SocketAddress, remoteAddress: SocketAddress, proto: MappedPort.Proto, callOptions: CallOptions? = nil) throws -> Int32 {
		let listener = try CakeAgentTunnelListener(group: self.eventLoopGroup, cakeAgentClient: client)

		_ = try listener.addPortForwardingServer(bindAddress: bindAddress, remoteAddress: remoteAddress, proto: proto, ttl: 5)

		try listener.bind().wait()
		return 0
	}

	public func events(callOptions: CallOptions? = nil, handler: @escaping (CakeAgent.TunnelPortForwardEvent.ForwardEvent) -> Void) throws {
		let stream: ServerStreamingCall<Cakeagent_CakeAgent.Empty, CakeAgent.TunnelPortForwardEvent>
		var subchannel: Channel? = nil

		stream = client.events(.init(), callOptions: callOptions) { event in
			if case let .forwardEvent(event) = event.event {
				handler(event)
			} else if case let .error(error) = event.event {
				//	throw error
				if let subchannel = subchannel {
					#if TRACE
						redbold("Event error: \(error)")
					#endif
					subchannel.pipeline.fireErrorCaught(GRPCStatus(code: .internalError, message: error))
				}
			}
		}

		subchannel = try stream.subchannel.wait()

		stream.status.whenComplete { result in
			switch result {
			case .failure(let err):
				#if TRACE
					redbold("Event error: \(err)")
				#endif
				subchannel?.close(promise: nil)
			case .success(let status):
				#if TRACE
					redbold("Tunnel status: \(status)")
				#endif
				if status.code != .ok {
					subchannel?.close(promise: nil)
				}
			}
		}

	}
}
