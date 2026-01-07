import Foundation
import GRPC
import NIOCore
import NIOPortForwarding
import NIOPosix
import ArgumentParser

enum PortForwardingServerError: Error {
	case notFound
}

public class CakeAgentPortForwardingServer {
	private static var portForwardingServer: CakeAgentPortForwardingServer! = nil

	private let mainGroup: EventLoopGroup
	private let ttl: Int
	private let bindAddresses: [String]
	private let remoteAddress: String
	private let portForwarder: CakeAgentPortForwarder
	private let dynamicPortForwarding: Bool
	private var closeFuture: PortForwarderClosure! = nil

	deinit {
		try? portForwarder.close()
	}

	private init(group: EventLoopGroup, cakeAgentClient: CakeAgentClient, bindAddresses: [String] = ["0.0.0.0", "[::]"], remoteAddress: String, forwardedPorts: [TunnelAttachement], dynamicPortForwarding: Bool, ttl: Int = 5) throws {
		self.mainGroup = group
		self.bindAddresses = bindAddresses
		self.remoteAddress = remoteAddress
		self.ttl = ttl
		self.dynamicPortForwarding = dynamicPortForwarding
		self.portForwarder = try CakeAgentPortForwarder(group: group, cakeAgentClient: cakeAgentClient, bindAddress: bindAddresses, remoteHost: remoteAddress, forwardedPorts: forwardedPorts, ttl: ttl)
	}

	@discardableResult
	private func bind() throws -> PortForwarderClosure {
		guard self.closeFuture == nil else {
			return self.closeFuture!
		}

		self.closeFuture = try self.portForwarder.bind()

		if self.dynamicPortForwarding {
			try self.portForwarder.startDynamicPortForwarding()
		}

		return self.closeFuture!
	}

	private func close() throws {
		Logger(self).info("Closing port forwarder")

		try portForwarder.close()

		Logger(self).info("Waiting for port forwarder to close")
		// Wait for the close future to complete
		try self.closeFuture?.wait()
		Logger(self).info("Port forwarder closed")
	}

	private func add(forwardedPorts: [ForwardedPort]) throws -> [any PortForwarding] {
		try self.portForwarder.addPortForwardingServer(
			remoteHost: self.remoteAddress,
			mappedPorts: forwardedPorts.map { MappedPort(host: $0.host, guest: $0.guest, proto: $0.proto) },
			bindAddress: self.bindAddresses,
			udpConnectionTTL: self.ttl)
	}

	private func delete(forwardedPorts: [ForwardedPort]) throws {
		try self.bindAddresses.forEach { bindAddress in
			try forwardedPorts.forEach {
				let bindAddress = try SocketAddress.makeAddress("tcp://\(bindAddress):\($0.host)")
				let remoteAddress = try SocketAddress.makeAddress("tcp://\(self.remoteAddress):\($0.guest)")

				do {
					try self.portForwarder.removePortForwardingServer(bindAddress: bindAddress, remoteAddress: remoteAddress, proto: $0.proto, ttl: self.ttl)
				} catch (PortForwardingError.alreadyBinded(let error)) {
					Logger(self).error(error)
				}
			}
		}
	}

	@discardableResult
	public static func createPortForwardingServer(group: EventLoopGroup, cakeAgentClient: CakeAgentClient, name: String, remoteAddress: String, forwardedPorts: [TunnelAttachement], dynamicPortForwarding: Bool) throws -> PortForwarderClosure {
		guard let server = portForwardingServer else {
			Logger(self).info("Configure forwarding ports for VM \(name)")

			portForwardingServer = try CakeAgentPortForwardingServer(group: group, cakeAgentClient: cakeAgentClient, remoteAddress: remoteAddress, forwardedPorts: forwardedPorts, dynamicPortForwarding: dynamicPortForwarding)

			let result = try portForwardingServer.bind()

			Logger(self).info("Port forwarding server created")

			return result
		}

		if case .stopped = server.portForwarder.status {
			Logger(self).info("Port forwarding server is stopped, restarting")
			return try server.bind()
		}

		throw ValidationError("Already configuring forwarded ports")
	}

	public static func removeForwardedPort(forwardedPorts: [ForwardedPort]) throws {
		if forwardedPorts.count > 0 {
			if let server = portForwardingServer {
				Logger(self).info("Remove forwarded ports \(forwardedPorts.map { $0.description }.joined(separator: ", "))")

				try server.delete(forwardedPorts: forwardedPorts)
			}
		}
	}

	public static func addForwardedPort(forwardedPorts: [ForwardedPort]) throws -> [any PortForwarding] {
		if forwardedPorts.count > 0 {
			if let portForwardingServer = portForwardingServer {
				Logger(self).info("Add forwarded ports \(forwardedPorts.map { $0.description }.joined(separator: ", "))")

				return try portForwardingServer.add(forwardedPorts: forwardedPorts)
			}
		}

		return []
	}

	public static func closeForwardedPort() throws {
		if let server = portForwardingServer {
			Logger(self).info("Close forwarded ports")

			defer {
				portForwardingServer = nil
			}

			try server.close()
		}
	}
}
