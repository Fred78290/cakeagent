import ArgumentParser
@preconcurrency import GRPC
import Foundation
import NIO
import NIOPosix
import NIOSSL

public typealias CakeAgentClient = Cakeagent_AgentNIOClient

extension CakeAgentClient {
	public func close() async throws {
		try await self.channel.close().get()
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
		self.client = try! CakeAgentHelper.createClient(on: on,
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

	public func info(callOptions: CallOptions? = nil) async throws -> String {
		let response = client.info(.init(), callOptions: callOptions)

		return try await response.response.get().jsonString()
	}

	public func exec(arguments: [String],
	                 inputHandle: FileHandle = FileHandle.standardInput,
	                 outputHandle: FileHandle = FileHandle.standardOutput,
	                 errorHandle: FileHandle = FileHandle.standardError,
	                 callOptions: CallOptions? = nil) async throws -> Int32 {
		let response = try await client.execute(Cakeagent_ExecuteRequest.with { req in
			if isatty(inputHandle.fileDescriptor) == 0{
				req.input = inputHandle.readDataToEndOfFile()
			}

			req.command = arguments.joined(separator: " ")
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

		shellStream = client.shell(callOptions: callOptions, handler: { response in
			if response.format == .stdout {
				outputHandle.write(response.datas)
			} else if response.format == .stderr {
				errorHandle.write(response.datas)
			} else {
				if let channel = pipeChannel {					
					_ = channel.channel.close()
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
							errorHandle.write("error: \(error)\n".data(using: .utf8)!)
							return
						}
					}
				}
				_ = stream.sendEnd()
			}
		}
	}
}