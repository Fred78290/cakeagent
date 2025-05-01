//
//  TTY.swift
//  CakeAgent
//
//  Created by Frederic BOLTZ on 09/03/2025.
//
import Foundation
import NIO
import GRPC
import CakeAgentLib
import System

struct Pipe {
	let fileHandleForReading: FileHandle
	let fileHandleForWriting: FileHandle
	
	init() {
		var fds: [Int32] = [-1, -1]
		if pipe(&fds) != 0 {
			fatalError("pipe failed")
		}
		
		fileHandleForReading = FileHandle(fileDescriptor: fds[0], closeOnDealloc: true)
		fileHandleForWriting = FileHandle(fileDescriptor: fds[1], closeOnDealloc: true)
	}
	
	func close() {
		try? self.fileHandleForWriting.close()
		try? self.fileHandleForReading.close()
	}
}

extension FileHandle {
	func dupCloseExec() -> FileHandle {
		return FileHandle(fileDescriptor: fcntl(self.fileDescriptor, F_DUPFD_CLOEXEC, 0), closeOnDealloc: true)
	}
	
	@discardableResult
	func setCloseExec(cloexec: Bool) throws -> FileHandle {
		let fd = self.fileDescriptor
		let flags = fcntl(fd, F_GETFD, 0)
		
		if flags < 0 {
			throw Errno(rawValue: errno)
		}
		
		if cloexec {
			if fcntl(fd, F_SETFD, flags|FD_CLOEXEC) < 0 {
				throw Errno(rawValue: errno)
			}
		} else {
			if fcntl(fd, F_SETFD, flags & ~FD_CLOEXEC) < 0 {
				throw Errno(rawValue: errno)
			}
		}

		return self
	}

	@discardableResult
	func SetNonblock(nonblocking: Bool) throws -> FileHandle {
		let fd = self.fileDescriptor
		let flags = fcntl(fd, F_GETFD, 0)

		if flags < 0 {
			throw Errno(rawValue: errno)
		}

		if (flags & O_NONBLOCK != 0) == nonblocking {
			return self
		}

		if nonblocking {
			if fcntl(fd, F_SETFD, flags|O_NONBLOCK) < 0 {
				throw Errno(rawValue: errno)
			}
		} else {
			if fcntl(fd, F_SETFD, flags & ~O_NONBLOCK) < 0 {
				throw Errno(rawValue: errno)
			}
		}
		
		return self
	}
}

class TTY: @unchecked Sendable {
	let isTTY: Bool
	private let ptx: FileHandle?
	private let pty: FileHandle?
	private let stdin: Pipe?
	private let stdout: Pipe?
	private let stderr: Pipe
	private var receivedBytes = 0

	public static func newPipe(nonblocking: Bool) throws -> Pipe {
		let pipe = Pipe()

		try pipe.fileHandleForReading.SetNonblock(nonblocking: nonblocking)
		try pipe.fileHandleForReading.setCloseExec(cloexec: true)
		try pipe.fileHandleForWriting.setCloseExec(cloexec: true)

		return pipe
	}

	var fileHandleForStdinWriting: FileHandle {
		if self.isTTY {
			return self.ptx!
		} else {
			return self.stdin!.fileHandleForWriting
		}
	}

	var fileHandleForStdinReading: FileHandle {
		if self.isTTY {
			return self.pty!
		} else {
			return self.stdin!.fileHandleForReading
		}
	}

	var fileHandleForStdoutReading: FileHandle {
		if isTTY {
			return self.ptx!
		} else {
			return self.stdout!.fileHandleForReading
		}
	}

	var fileHandleForStderrReading: FileHandle {
		return self.stderr.fileHandleForReading
	}

	var fileHandleForStdoutWriting: FileHandle {
		if isTTY {
			return self.pty!
		} else {
			return stdout!.fileHandleForWriting
		}
	}

	var fileHandleForStderrWriting: FileHandle {
		return self.stderr.fileHandleForWriting
	}

	init(tty: Bool) throws {
		self.isTTY = tty

		if self.isTTY {
			let master = try Self.createPTY()

			self.ptx = FileHandle(fileDescriptor: master.0, closeOnDealloc: true)
			self.pty = FileHandle(fileDescriptor: master.1, closeOnDealloc: true)
			self.stdin = nil
			self.stdout = nil
		} else {
			self.ptx = nil
			self.pty = nil
			self.stdin = try Self.newPipe(nonblocking: false)
			self.stdout = try Self.newPipe(nonblocking: true)
		}

		self.stderr = try Self.newPipe(nonblocking: true)
	}

	func setTermSize(rows: Int32, cols: Int32) throws {
		if isTTY {
			try pty!.setTermSize(rows: rows, cols: cols)
		}
	}

	func eof() {
		if Logger.Level() >= .debug {
			Logger(self).debug("received EOF")
		}

		if let ptx = self.ptx {
			ptx.closeFile()
		} else if let stdin = self.stdin {
			stdin.fileHandleForWriting.closeFile()
			//stdin.fileHandleForReading.closeFile()
		}
	}

	func close() {
		if Logger.Level() >= .debug {
			Logger(self).debug("close TTY")
		}

		if isTTY {
			self.ptx!.closeFile()
			self.pty!.closeFile()
		} else {
			self.stdin!.close()
			self.stdout!.close()
		}
		
		self.stderr.close()
	}

	func writeToStdin(_ data: Data) {
		receivedBytes += data.count

		if Logger.Level() >= .trace {
			Logger(self).trace("stdin: \(data.count), receivedBytes: \(receivedBytes) [\(String(data: data, encoding: .utf8) ?? "<unknown>")]")
		} else if Logger.Level() >= .debug {
			Logger(self).debug("stdin: \(data.count), receivedBytes: \(receivedBytes)")
		}

		self.fileHandleForStdinWriting.write(data)
	}

	@discardableResult
	func bootstrap(group: EventLoopGroup) async throws -> CakeAgentExecuteStream {
		if self.isTTY {
			try await NIOPipeBootstrap(group: group)
				.takingOwnershipOfDescriptor(inputOutput: dup(self.ptx!.fileDescriptor)) { channel in
					return channel.eventLoop.makeCompletedFuture {
						try NIOAsyncChannel(wrappingChannelSynchronously: channel, configuration: .init(isOutboundHalfClosureEnabled: true))
					}
				}
		} else {
			try await NIOPipeBootstrap(group: group)
				.takingOwnershipOfDescriptors(input: dup(self.stdout!.fileHandleForReading.fileDescriptor), output: dup(self.stdin!.fileHandleForWriting.fileDescriptor)) { channel in
					return channel.eventLoop.makeCompletedFuture {
						try NIOAsyncChannel(wrappingChannelSynchronously: channel, configuration: .init(isOutboundHalfClosureEnabled: true))
					}
				}
		}
	}

	private static func createPTY() throws -> (Int32, Int32) {
		var tty_fd: Int32 = -1
		var sfd: Int32 = -1
		let tty_path = UnsafeMutablePointer<CChar>.allocate(capacity: 1024)

		defer {
			tty_path.deallocate()
		}

		let res = openpty(&tty_fd, &sfd, tty_path, nil, nil);

		if (res < 0) {
			throw Errno(rawValue: errno)
		}

		return (tty_fd, sfd)
	}
}

