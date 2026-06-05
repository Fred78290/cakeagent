import Foundation
import ArgumentParser
import NIO

extension FileManager {
	public static var realHomeDirectoryForCurrentUser: URL {
		let home: String = getpwuid(getuid()).flatMap { pw in
			pw.pointee.pw_dir.map { String(cString: $0) }
		} ?? NSHomeDirectory()

		return URL(fileURLWithPath: home, isDirectory: true)
	}
}

extension String {
	var expandingTildeInPath: String {
		if self.hasPrefix("~") {
			let home = FileManager.realHomeDirectoryForCurrentUser.path(percentEncoded: false)
			let name = self.dropFirst(1)

			if name.isEmpty {
				return home
			}

			return home + name
		}
		
		return self
	}

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