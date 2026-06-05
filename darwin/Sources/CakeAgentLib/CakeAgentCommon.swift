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
		guard self.hasPrefix("~") else {
			return self
		}

		let currentHome = FileManager.realHomeDirectoryForCurrentUser.path(percentEncoded: false)

		// "~" or "~/..."
		if self == "~" {
			return currentHome
		}

		if self.hasPrefix("~/") {
			return currentHome + String(self.dropFirst())
		}

		// "~user" or "~user/..."
		let afterTilde = self.index(after: self.startIndex)
		let slashIndex = self[afterTilde...].firstIndex(of: "/")
		let userEnd = slashIndex ?? self.endIndex
		let user = String(self[afterTilde..<userEnd])
		let rest = slashIndex.map { String(self[$0...]) } ?? ""

		guard let pw = getpwnam(user), let dir = pw.pointee.pw_dir else {
			return self
		}

		return String(cString: dir) + rest
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