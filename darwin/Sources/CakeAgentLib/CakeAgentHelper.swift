import ArgumentParser
@preconcurrency import GRPC
import Foundation
import NIO
import NIOPosix
import NIOSSL

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

public struct InfoReply: Sendable, Codable {
	public var name: String
	public var version: String?
	public var uptime: UInt64?
	public var memory: MemoryInfo?
	public var cpuCount: Int32
	public var ipaddresses: [String]
	public var osname: String
	public var hostname: String?
	public var release: String?
	public var mounts: [String]?
	public var status: Status

	init() {
		self.name = ""
		self.version = nil
		self.uptime = 0
		self.memory = nil
		self.cpuCount = 0
		self.ipaddresses = []
		self.osname = ""
		self.hostname = nil
		self.release = nil
		self.mounts = nil
		self.status = .stopped
	}

	public init(info: Cakeagent_InfoReply) {
		self.name = info.hostname
		self.version = info.version
		self.uptime = info.uptime
		self.cpuCount = info.cpuCount
		self.ipaddresses = info.ipaddresses
		self.osname = info.osname
		self.hostname = info.hostname
		self.release = info.release
		self.status = .running

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
	var pipeChannel: NIOAsyncChannel<ByteBuffer, ByteBuffer>? = nil
	var exitCode: Int32 = 0

	init(on: EventLoop, inputHandle: FileHandle, outputHandle: FileHandle, errorHandle: FileHandle) {
		self.eventLoop = on
		self.inputHandle = inputHandle
		self.outputHandle = outputHandle
		self.errorHandle = errorHandle
	}
	
	func handleResponse(response: Cakeagent_ShellResponse) -> Void {
		if let channel = self.pipeChannel {
			if case let .exitCode(code) = response.response {
				self.exitCode = code
				_ = channel.channel.close()
			} else {
				channel.channel.eventLoop.execute {
					do {
						if case let .stdout(datas) = response.response {
							try self.outputHandle.write(contentsOf: datas)
						} else if case let .stderr(datas) = response.response {
							try self.errorHandle.write(contentsOf: datas)
						}
					} catch {
						if error is CancellationError == false {
							guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
								let errMessage = "error: \(error)\n".data(using: .utf8)!
								
								FileHandle.standardError.write(errMessage)
								self.errorHandle.write(errMessage)
								return
							}
						}
					}
				}
			}
			
		}
	}

	func stream(handler: @escaping () -> BidirectionalStreamingCall<Cakeagent_ShellRequest, Cakeagent_ShellResponse>) async throws -> Int32 {
		let stream = handler()

		let sigwinch = DispatchSource.makeSignalSource(signal: SIGWINCH)

		sigwinch.setEventHandler {
			stream.eventLoop.execute {
				let size = self.outputHandle.getTermSize()
				let message = Cakeagent_ShellRequest.with {
					$0.input = Cakeagent_ShellMessage.with {
						$0.rows = size.rows
						$0.cols = size.cols
					}
				}

				_ = stream.sendMessage(message)
			}
		}

		sigwinch.activate()

		defer {
			sigwinch.cancel()
			_ = stream.sendEnd()
		}

		pipeChannel = try await stream.subchannel.flatMapThrowing { streamChannel in
			return Task {
				return try await NIOPipeBootstrap(group: self.eventLoop)
					.takingOwnershipOfDescriptor(input: dup(self.inputHandle.fileDescriptor)) { pipeChannel in
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
					let size = outputHandle.getTermSize()
					let message = Cakeagent_ShellRequest.with {
						$0.input = Cakeagent_ShellMessage.with {
							$0.datas = Data(buffer: buffer)
							$0.rows = size.rows
							$0.cols = size.cols
						}
					}

					_ = stream.sendMessage(message)
				}
			} catch {
				if error is CancellationError == false {
					guard let err = error as? ChannelError, err == ChannelError.ioOnClosedChannel else {
						let errMessage = "error: \(error)\n".data(using: .utf8)!

						FileHandle.standardError.write(errMessage)
						errorHandle.write(errMessage)
						return
					}
				}
			}
		}
		
		return self.exitCode
	}
}

public struct CakeAgentHelper: Sendable {
	internal let eventLoopGroup: EventLoopGroup
	internal let client: CakeAgentClient

	public init(on: EventLoopGroup, client: CakeAgentClient) {
		self.eventLoopGroup = on
		self.client = client
	}

	public init(on: EventLoopGroup, listeningAddress: URL, connectionTimeout: Int64, caCert: String?, tlsCert: String?, tlsKey: String?) throws {
		self.eventLoopGroup = on
		self.client = try! Self.createClient(on: on,
		                                     listeningAddress: listeningAddress,
		                                     connectionTimeout: connectionTimeout,
		                                     caCert: caCert,
		                                     tlsCert: tlsCert,
		                                     tlsKey: tlsKey)
	}

	public static func createClient(on: EventLoopGroup,
	                                listeningAddress: URL,
	                                connectionTimeout: Int64,
	                                caCert: String?,
	                                tlsCert: String?,
	                                tlsKey: String?,
	                                retries: ConnectionBackoff.Retries = .unlimited) throws -> CakeAgentClient {
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

		return CakeAgentClient(channel: ClientConnection(configuration: clientConfiguration))
	}

	public func info(callOptions: CallOptions? = nil) throws -> InfoReply {
		let response = client.info(.init(), callOptions: callOptions)
		let infos = try response.response.wait()

		return InfoReply.with {
			$0.name = infos.hostname
			$0.version = infos.version
			$0.uptime = infos.uptime
			$0.cpuCount = infos.cpuCount
			$0.ipaddresses = infos.ipaddresses
			$0.osname = infos.osname
			$0.hostname = infos.hostname
			$0.release = infos.release
			$0.status = .running
			$0.memory = infos.hasMemory ? InfoReply.MemoryInfo.with {
				$0.total = infos.memory.total
				$0.free = infos.memory.free
				$0.used = infos.memory.used
			} : nil
		}
	}

	public func exec(command: String,
	                 arguments: [String],
	                 inputHandle: FileHandle = FileHandle.standardInput,
	                 outputHandle: FileHandle = FileHandle.standardOutput,
	                 errorHandle: FileHandle = FileHandle.standardError,
	                 callOptions: CallOptions? = nil) async throws -> Int32 {
		let size = outputHandle.getTermSize()

		let handler = CakeChannelStreamer(on: self.eventLoopGroup.next(), inputHandle: inputHandle, outputHandle: outputHandle, errorHandle: errorHandle)

		return try await handler.stream {
			let stream = client.execute(callOptions: callOptions, handler: handler.handleResponse)
			let cmd = Cakeagent_ShellRequest.with {
				$0.command = Cakeagent_ExecuteCommand.with {
					$0.command = command
					$0.args = arguments
					$0.rows = size.rows
					$0.cols = size.cols
				}
			}

			_ = stream.sendMessage(cmd)

			return stream
		}
	}

	public func shell(inputHandle: FileHandle = FileHandle.standardInput,
	                  outputHandle: FileHandle = FileHandle.standardOutput,
	                  errorHandle: FileHandle = FileHandle.standardError,
	                  callOptions: CallOptions? = nil) async throws -> Int32 {
		var term = inputHandle.makeRaw()

		defer {
			inputHandle.restoreState(&term)
		}

		let handler = CakeChannelStreamer(on: self.eventLoopGroup.next(), inputHandle: inputHandle, outputHandle: outputHandle, errorHandle: errorHandle)

		return try await handler.stream {
			client.shell(callOptions: callOptions, handler: handler.handleResponse)
		}
	}

	public func mount(request: Cakeagent_MountRequest, callOptions: CallOptions? = nil) throws -> Cakeagent_MountReply {
		let response = client.mount(request, callOptions: callOptions)

		return try response.response.wait()
	}

	public func umount(request: Cakeagent_MountRequest, callOptions: CallOptions? = nil) throws -> Cakeagent_MountReply {
		let response = client.umount(request, callOptions: callOptions)

		return try response.response.wait()
	}
}
