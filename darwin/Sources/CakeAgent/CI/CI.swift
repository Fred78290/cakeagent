struct CI {
	private static let rawVersion = ""

	static var version: String {
		rawVersion.expanded() ? rawVersion : "SNAPSHOT"
	}

	static var release: String? {
		rawVersion.expanded() ? "cakeagent@\(rawVersion)" : nil
	}
}

private extension String {
	func expanded() -> Bool {
		!isEmpty && !starts(with: "$")
	}
}
