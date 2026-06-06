import Foundation
import ArgumentParser
import NIO

extension String {
	public func toSocketAddress() throws -> SocketAddress {
		let socketAddress: SocketAddress

		guard let u = URL(string: self) else {
			throw ValidationError("protocol error, expected TunnelMessageConnect.guestAddress to be a valid URL")
		}

		if u.scheme == "unix" || u.isFileURL {
			socketAddress = try .init(unixDomainSocketPath: u.path)
		} else {
			socketAddress = try .init(ipAddress: u.host ?? "127.0.0.1", port: Int(u.port ?? 0))
		}

		return socketAddress
	}
}