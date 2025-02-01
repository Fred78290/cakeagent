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
}

public struct InfoReply: Sendable, Codable {
	public let version: String
	public let uptime: Int64
	public let memory: MemoryInfo?
	public let cpuCount: Int32
	public let ipaddresses: [String]
	public let osname: String
	public let hostname: String
	public let release: String 

	public struct MemoryInfo: Sendable, Codable {
		public let total: UInt64
		public let free: UInt64
		public let used: UInt64
	}

	public func toJSON() -> String {
		let encoder = JSONEncoder()
		encoder.outputFormatting = .prettyPrinted

		return String(data: try! encoder.encode(self), encoding: .utf8)!
	}
}

extension FileHandle {
	func makeRaw() -> termios {
		var term: termios = termios()
		let inputTTY: Bool = isatty(self.fileDescriptor) != 0

		if inputTTY {
			if tcgetattr(self.fileDescriptor, &term) != 0 {
				perror("tcgetattr error")
			}

			var newState: termios = term

			newState.c_iflag &= UInt(IGNBRK) | ~UInt(BRKINT | INPCK | ISTRIP | IXON)
			newState.c_cflag |= UInt(CS8)
			newState.c_lflag &= ~UInt(ECHO | ICANON | IEXTEN | ISIG)
			newState.c_cc.16 = 1
			newState.c_cc.17 = 17

			if tcsetattr(self.fileDescriptor, TCSANOW, &newState) != 0 {
				perror("tcsetattr error")
			}
		}

		return term
	}

	func restoreState(_ term: UnsafePointer<termios>) {
		if tcsetattr(self.fileDescriptor, TCSANOW, term) != 0 {
			perror("tcsetattr error")
		}
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
	                                tlsKey: String?) throws -> CakeAgentClient {
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

		clientConfiguration.connectionBackoff = ConnectionBackoff(maximumBackoff: TimeInterval(connectionTimeout), minimumConnectionTimeout: 5.0)

		return CakeAgentClient(channel: ClientConnection(configuration: clientConfiguration))
	}

	public func info(callOptions: CallOptions? = nil) async throws -> InfoReply {
		let response = client.info(.init(), callOptions: callOptions)
		let infos = try await response.response.get()

		return InfoReply(version: infos.version,
		                 uptime: infos.uptime,
		                 memory: infos.hasMemory ? InfoReply.MemoryInfo(total: infos.memory.total,
		                                                                free: infos.memory.free,
		                                                                used: infos.memory.used) : nil,
		                 cpuCount: infos.cpuCount,
		                 ipaddresses: infos.ipaddresses,
		                 osname: infos.osname,
		                 hostname: infos.hostname,
		                 release: infos.release)
	}

	public func exec(command: String,
					 arguments: [String],
	                 inputHandle: FileHandle = FileHandle.standardInput,
	                 outputHandle: FileHandle = FileHandle.standardOutput,
	                 errorHandle: FileHandle = FileHandle.standardError,
	                 callOptions: CallOptions? = nil) async throws -> Int32 {
		var state = inputHandle.makeRaw()

		defer {
			inputHandle.restoreState(&state)
		}

		let response = try await client.execute(Cakeagent_ExecuteRequest.with { req in
			if isatty(inputHandle.fileDescriptor) == 0 {
				req.input = inputHandle.readDataToEndOfFile()
			}

			req.command = command
			req.args = arguments
		}).response.get()

		if response.hasError {
			errorHandle.write(response.error)
		}

		if response.hasOutput {
			outputHandle.write(response.output)
		}

		return response.exitCode
	}

	public func shell(inputHandle: FileHandle = FileHandle.standardInput,
	                  outputHandle: FileHandle = FileHandle.standardOutput,
	                  errorHandle: FileHandle = FileHandle.standardError,
	                  callOptions: CallOptions? = nil) async throws {
		var shellStream: BidirectionalStreamingCall<Cakeagent_ShellMessage, Cakeagent_ShellResponse>?
		var pipeChannel: NIOAsyncChannel<ByteBuffer, ByteBuffer>?
		var term = inputHandle.makeRaw()

		defer {
			inputHandle.restoreState(&term)
		}

		shellStream = client.shell(callOptions: callOptions, handler: { response in
			if let channel = pipeChannel {					
				if response.format == .end {
					_ = channel.channel.close()
				} else {
					channel.channel.eventLoop.execute {
						do {
							if response.format == .stdout {
								try outputHandle.write(contentsOf: response.datas)
							} else if response.format == .stderr {
								try errorHandle.write(contentsOf: response.datas)
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
				}
			}
		})

		if let stream = shellStream {
			pipeChannel = try await stream.subchannel.flatMapThrowing { streamChannel in

				return Task {
					return try await NIOPipeBootstrap(group: self.eventLoopGroup)
						.takingOwnershipOfDescriptor(input: dup(inputHandle.fileDescriptor)) { pipeChannel in
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
							let errMessage = "error: \(error)\n".data(using: .utf8)!

							FileHandle.standardError.write(errMessage)
							errorHandle.write(errMessage)
							return
						}
					}
				}
				_ = stream.sendEnd()
			}
		}
	}
}
