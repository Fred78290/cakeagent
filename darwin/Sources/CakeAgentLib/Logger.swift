import ArgumentParser
import Foundation
import Logging

extension Logging.Logger.Level {
	public var level: Logger.LogLevel {
		switch self {
		case .trace:
			return .trace
		case .debug:
			return .debug
		case .info:
			return .info
		case .notice:
			return .notice
		case .warning:
			return .warning
		case .error:
			return .error
		case .critical:
			return .critical
		}
	}
}

public final class Logger {
	public enum LogLevel: Int, ExpressibleByArgument, Equatable, Comparable, CustomStringConvertible {
		case trace = 6
		case debug = 5
		case info = 4
		case notice = 3
		case warning = 2
		case error = 1
		case critical = 0

		public var level: Logging.Logger.Level {
			switch self {
			case .trace:
				return .trace
			case .debug:
				return .debug
			case .info:
				return .info
			case .notice:
				return .notice
			case .warning:
				return .warning
			case .error:
				return .error
			case .critical:
				return .critical
			}
		}

		public var description: String {
			switch self {
				case .critical:
					return "critical"
				case .error:
					return "error"
				case .warning:
					return "warning"
				case .notice:
					return "notice"
				case .info:
					return "info"
				case .debug:
					return "debug"
				case .trace:
					return "trace"
			}
		}

		public static func < (lhs: LogLevel, rhs: LogLevel) -> Bool {
			return lhs.rawValue < rhs.rawValue
		}

		public init?(argument: String) {
			switch argument {
			case "trace":
				self = .trace
			case "debug":
				self = .debug
			case "info":
				self = .info
			case "notice":
				self = .notice
			case "warning":
				self = .warning
			case "error":
				self = .error
			case "critical":
				self = .critical
			default:
				return nil
			}
		}
	}

	private static let eraseCursorDown: String = "\u{001B}[J"
	private static let moveUp = "\u{001B}[1A"
	private static let moveBeginningOfLine = "\r"
	private static var logLevel = LogLevel.info

	private let label = "com.aldunelabs"
	private var logger: Logging.Logger
	private let isTTY: Bool

	public init(_ target: Any) {
		let thisType = type(of: target)
		self.logger = Logging.Logger(label: "com.aldunelabs.\(String(describing: thisType))")
		self.logger.logLevel = Self.logLevel.level
		self.isTTY = FileHandle.standardOutput.isTTY()
	}

	public init(_ label: String) {
		self.logger = Logging.Logger(label: "com.aldunelabs.\(label)")
		self.logger.logLevel = Self.logLevel.level
		self.isTTY = FileHandle.standardOutput.isTTY()
	}

	static public func Level() -> LogLevel {
		Self.logLevel
	}

	static public func LoggingLevel() -> Logging.Logger.Level {
		Self.logLevel.level
	}

	static public func setLevel(_ level: LogLevel) {
		Self.logLevel = level
	}

	public func error(_ err: Error) {
		if Self.logLevel >= LogLevel.error {
			if self.isTTY {
				logger.error("\u{001B}[0;31m\u{001B}[1m\(String(stringLiteral: err.localizedDescription))\u{001B}[0m")
			} else {
				logger.error(.init(stringLiteral: err.localizedDescription))
			}
		}
	}

	public func error(_ err: String) {
		if Self.logLevel >= LogLevel.error {
			if self.isTTY {
				logger.error("\u{001B}[0;31m\u{001B}[1m\(String(stringLiteral: err))\u{001B}[0m")
			} else {
				logger.error(.init(stringLiteral: err))
			}
		}
	}

	public func warn(_ line: String) {
		if Self.logLevel >= LogLevel.warning {
			if self.isTTY {
				logger.warning("\u{001B}[0;33m\u{001B}[1m\(String(stringLiteral: line))\u{001B}[0m")
			} else {
				logger.warning(.init(stringLiteral: line))
			}
		}
	}

	public func info(_ line: String) {
		if Self.logLevel >= LogLevel.info {
			logger.info(.init(stringLiteral: line))
		}
	}

	public func debug(_ line: String) {
		if Self.logLevel >= LogLevel.debug {
			if self.isTTY {
				logger.debug("\u{001B}[0;32m\u{001B}[1m\(String(stringLiteral: line))\u{001B}[0m")
			} else {
				logger.debug(.init(stringLiteral: line))
			}
		}
	}

	public func trace(_ line: String) {
		if Self.logLevel >= LogLevel.trace {
			if self.isTTY {
				logger.trace("\u{001B}[0;34m\u{001B}[1m\(String(stringLiteral: line))\u{001B}[0m")
			} else {
				logger.trace(.init(stringLiteral: line))
			}
		}
	}

	static public func appendNewLine(_ line: String) {
		if line.isEmpty {
			return
		}

		print(line, terminator: "\n")
	}

	static public func updateLastLine(_ line: String) {
		if line.isEmpty {
			return
		}

		print(moveUp, moveBeginningOfLine, eraseCursorDown, line, separator: "", terminator: "\n")
	}
}
