@preconcurrency import GRPC
import Foundation
import NIO
import NIOPosix
import NIOSSL
import Semaphore
import Synchronization
import Logging
import NIOPortForwarding
import CryptoKit

open class CakeAgentTunnelListener: PortForwarder, @unchecked Sendable {
	internal let cakeAgentClient: CakeAgentClient

	open class CakeAgentChannelTunnelHandlerAdapter: ChannelInboundHandler {
		public typealias InboundIn = ByteBuffer
		public typealias OutboundOut = ByteBuffer

		let cakeAgentClient: CakeAgentClient
		let bindAddress: SocketAddress
		let remoteAddress: SocketAddress
		let proto: CakeAgent.TunnelMessage.TunnelProtocol

		var tunnel: BidirectionalStreamingCall<Cakeagent_CakeAgent.TunnelMessage, Cakeagent_CakeAgent.TunnelMessage>? = nil

		public init(proto: CakeAgent.TunnelMessage.TunnelProtocol, bindAddress: SocketAddress, remoteAddress: SocketAddress, cakeAgentClient: CakeAgentClient) {
			self.proto = proto
			self.bindAddress = bindAddress
			self.remoteAddress = remoteAddress
			self.cakeAgentClient = cakeAgentClient
		}

		public func channelRegistered(context: ChannelHandlerContext) {
			#if TRACE
				redbold("Create tunnel from \(self.bindAddress) to \(self.remoteAddress)")
			#endif
			self.tunnel = cakeAgentClient.tunnel(callOptions: nil) { message in
				// Handle incoming messages from the tunnel
				#if TRACE
					redbold("Receive message \(message)")
				#endif
				switch message.message {
				case .datas(let data):
					// Send data to the channel
					let buffer = context.channel.allocator.buffer(data: data)
					context.channel.writeAndFlush(buffer, promise: nil)
				case .eof:
					// Handle tunnel close
					context.channel.close(promise: nil)
				case .error(let err):
					// Handle error
					#if TRACE
						redbold("Tunnel error: \(err)")
					#endif
					self.errorCaught(context: context, error: IOError(errnoCode: EIO, reason: err))
				default:
					break
				}
			}

			self.tunnel?.status.whenComplete { result in
				switch result {
				case .failure(let err):
#if TRACE
					redbold("Tunnel error: \(err)")
#endif
					self.errorCaught(context: context, error: err)
				case .success(let status):
#if TRACE
					redbold("Tunnel status: \(status)")
#endif
					if status.code != .ok {
						self.errorCaught(context: context, error: status)
					}
				}
			}

			context.fireChannelRegistered()
		}

		public func channelActive(context: ChannelHandlerContext) {
			if let tunnel = self.tunnel {
				let connect = CakeAgent.TunnelMessage.with {
					$0.connect = CakeAgent.TunnelMessage.TunnelMessageConnect.with { message in
						message.id = SHA256.hash(data: Data("\(self.bindAddress.description):\(self.remoteAddress.description)@\(context.channel)".utf8)).description
						message.protocol = self.proto
						message.guestAddress = self.remoteAddress.pathname!
					}
				}

				#if TRACE
					redbold("Connect tunnel \(connect)")
				#endif

				_ = tunnel.sendMessage(connect)
			}

			context.fireChannelActive()
		}

		public func channelInactive(context: ChannelHandlerContext) {
			if let tunnel = self.tunnel {
				#if TRACE
					redbold("Disconnect tunnel from \(self.bindAddress) to \(self.remoteAddress)")
				#endif
				tunnel.sendMessage(CakeAgent.TunnelMessage.with { $0.eof = true}).whenComplete { _ in
					_ = tunnel.sendEnd()
				}
			}

			context.fireChannelInactive()
		}

		public func channelRead(context: ChannelHandlerContext, data: NIOAny) {
			if let tunnel = self.tunnel {
				let data = self.unwrapInboundIn(data)

				let message = CakeAgent.TunnelMessage.with {
					$0.datas = Data(buffer: data)
				}

				#if TRACE
					redbold("Send message \(message)")
				#endif
				_ = tunnel.sendMessage(message)
			} else {
				context.fireChannelRead(data)
			}
		}

		public func errorCaught(context: ChannelHandlerContext, error: Error) {
			#if TRACE
				redbold("errorCaught \(error)")
			#endif
			context.close(promise: nil)

			context.fireErrorCaught(error)
		}
	}

	open class CakeAgentTCPPortForwardingServer: TCPPortForwardingServer {
		let cakeAgentClient: CakeAgentClient

		public init(on: EventLoop, bindAddress: SocketAddress, remoteAddress: SocketAddress, cakeAgentClient: CakeAgentClient) {
			self.cakeAgentClient = cakeAgentClient

			super.init(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress)
		}

		open override func childChannelInitializer(channel: Channel) -> EventLoopFuture<Void> {
			return channel.pipeline.addHandler(CakeAgentChannelTunnelHandlerAdapter(proto: .tcp, bindAddress: self.bindAddress, remoteAddress: self.remoteAddress, cakeAgentClient: self.cakeAgentClient))
		}
	}

	open class CakeAgentUDPPortForwardingServer: UDPPortForwardingServer {
		internal let cakeAgentClient: CakeAgentClient

		public init(on: EventLoop,
		            bindAddress: SocketAddress,
		            remoteAddress: SocketAddress,
		            ttl: Int,
		            cakeAgentClient: CakeAgentClient) {
			self.cakeAgentClient = cakeAgentClient

			super.init(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress, ttl: ttl)
		}

		open override func channelInitializer(channel: Channel) -> EventLoopFuture<Void> {
			return channel.pipeline.addHandler(CakeAgentChannelTunnelHandlerAdapter(proto: .udp, bindAddress: self.bindAddress, remoteAddress: self.remoteAddress, cakeAgentClient: self.cakeAgentClient))
		}
	}

	public init(group: EventLoopGroup, remoteHost: String = "127.0.0.1", bindAddress: String = "127.0.0.1", mappedPorts: [MappedPort] = [], ttl: Int = 5, cakeAgentClient: CakeAgentClient) throws {
		self.cakeAgentClient = cakeAgentClient

		try super.init(group: group, remoteHost: remoteHost, mappedPorts: mappedPorts, bindAddresses: [bindAddress], udpConnectionTTL: ttl)
	}

	open override func createTCPPortForwardingServer(on: EventLoop, bindAddress: SocketAddress, remoteAddress: SocketAddress) throws -> any PortForwarding {
		if remoteAddress.protocol == .unix {
			return CakeAgentTCPPortForwardingServer(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress, cakeAgentClient: cakeAgentClient)
		}

		return try super.createTCPPortForwardingServer(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress)
	}

	open override func createUDPPortForwardingServer(on: EventLoop, bindAddress: SocketAddress, remoteAddress: SocketAddress, ttl: Int) throws -> any PortForwarding {
		if remoteAddress.protocol == .unix {
			return CakeAgentUDPPortForwardingServer(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress, ttl: ttl, cakeAgentClient: cakeAgentClient)
		}

		return try super.createUDPPortForwardingServer(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress, ttl: ttl)
	}
}
