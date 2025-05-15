//
//  TunnelHandler.swift
//  CakeAgent
//
//  Created by Frederic BOLTZ on 08/05/2025.
//
import Foundation
import CakeAgentLib
import GRPC
import NIO

public typealias CakeAgentTunnelRequestStream = GRPCAsyncRequestStream<CakeAgent.TunnelMessage>
public typealias CakeAgentTunnelResponseStream = GRPCAsyncResponseStreamWriter<CakeAgent.TunnelMessage>

struct TunnelHandlerStream {
	let requestStream: CakeAgentTunnelRequestStream
	let responseStream: CakeAgentTunnelResponseStream
	let eventLoop: EventLoop
	
	protocol Connectable {
		func connect<Output: Sendable>(to: SocketAddress, channelInitializer: @escaping @Sendable (any Channel) -> EventLoopFuture<Output>) async throws -> Output
		func close() -> EventLoopFuture<Void>
	}

	final class TCPTunnelHandlerStream: Connectable {
		let on: EventLoop
		let bootstrap: ClientBootstrap
		var channel: Channel? = nil

		init(on: EventLoop) {
			self.on = on
			self.bootstrap = ClientBootstrap(group: on)
		}

		func connect<Output: Sendable>(to: SocketAddress, channelInitializer: @escaping @Sendable (any Channel) -> EventLoopFuture<Output>) async throws -> Output {
			return try await self.bootstrap.connect(to: to) { channel in
				self.channel = channel
				
				return channelInitializer(channel)
			}
		}
		
		func close() -> EventLoopFuture<Void> {
			if let channel = self.channel {
				return channel.close()
			} else {
				return self.on.makeSucceededVoidFuture()
			}
		}
	}

	final class UDPTunnelHandlerStream: Connectable {
		let on: EventLoop
		let bootstrap: DatagramBootstrap
		var channel: Channel? = nil

		init(on: EventLoop) {
			self.on = on
			self.bootstrap = DatagramBootstrap(group: on)
		}

		func connect<Output: Sendable>(to: SocketAddress, channelInitializer: @escaping @Sendable (any Channel) -> EventLoopFuture<Output>) async throws -> Output {
			return try await self.bootstrap.connect(to: to) { channel in
				self.channel = channel
				
				return channelInitializer(channel)
			}
		}
		
		func close() -> EventLoopFuture<Void> {
			if let channel = self.channel {
				return channel.close()
			} else {
				return self.on.makeSucceededVoidFuture()
			}
		}
	}

	init(on: EventLoop, requestStream: CakeAgentTunnelRequestStream, responseStream: CakeAgentTunnelResponseStream) {
		self.eventLoop = on
		self.requestStream = requestStream
		self.responseStream = responseStream
	}

	func stream() async throws {
		var iterator = requestStream.makeAsyncIterator()
		let client: Connectable
				
		guard let firstRequest = try await iterator.next(), case let .connect(connect) = firstRequest.message else {
			throw ServiceError("protocol error, expected TunnelMessageConnect")
		}
		
		let socketAddress = try connect.guestAddress.toSocketAddress()

		if connect.protocol == .tcp {
			client = TCPTunnelHandlerStream(on: self.eventLoop)
		} else {
			client = UDPTunnelHandlerStream(on: self.eventLoop)
		}
		
		let result = try await client.connect(to: socketAddress) { channel in
			return channel.eventLoop.makeCompletedFuture {
				try NIOAsyncChannel<ByteBuffer, ByteBuffer>(wrappingChannelSynchronously: channel)
			}
		}

		try await result.executeThenClose { input, output in
			await withTaskGroup { group in
				group.addTask {
					do {
						for try await data in input {
							do {
								try await self.responseStream.send(CakeAgent.TunnelMessage.with {
									$0.message = .datas(Data(buffer: data))
								})
							} catch {
								print("Error sending response: \(error)")
							}
						}
					} catch {
						Logger(self).error("Error reading input: \(error)")
					}

					try? await self.responseStream.send(CakeAgent.TunnelMessage.with {
						$0.message = .eof(true)
					})
				}

				group.addTask {
					do {
						while let request = try await iterator.next() {
							if case let .datas(data) = request.message {
								try await output.write(ByteBuffer(data: data))
							} else if case .eof = request.message {
								return
							} else {
								throw ServiceError("protocol error, unexpected TunnelMessage: \(String(describing: request.message))")
							}
						}
					} catch {
						Logger(self).error("Error reading request: \(error)")
					}
				}

				await group.waitForAll()

				try? await client.close().get()
			}
		}
	}
}
