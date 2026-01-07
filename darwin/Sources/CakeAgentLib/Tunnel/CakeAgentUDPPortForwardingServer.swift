import Foundation
import NIO
import NIOPortForwarding

class CakeAgentUDPPortForwardingServer: UDPPortForwardingServer {
	internal let cakeAgentClient: CakeAgentClient

	public init(
		on: EventLoop,
		bindAddress: SocketAddress,
		remoteAddress: SocketAddress,
		ttl: Int,
		cakeAgentClient: CakeAgentClient
	) {
		self.cakeAgentClient = cakeAgentClient

		super.init(on: on, bindAddress: bindAddress, remoteAddress: remoteAddress, ttl: ttl)
	}

	override func channelInitializer(channel: Channel) -> EventLoopFuture<Void> {
		return channel.pipeline.addHandler(CakeAgentChannelTunnelHandlerAdapter(proto: .udp, bindAddress: self.bindAddress, remoteAddress: self.remoteAddress, cakeAgentClient: self.cakeAgentClient))
	}
}
