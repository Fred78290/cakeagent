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

	var fileHandleForInput: FileHandle {
		if isTTY {
			return ptx!
		} else {
			return stdin!.fileHandleForWriting
		}
	}

	var fileHandleForOutput: FileHandle {
		if isTTY {
			return ptx!
		} else {
			return stdout!.fileHandleForReading
		}
	}

	var fileHandleForReading: FileHandle {
		if isTTY {
			return pty!
		} else {
			return stdin!.fileHandleForReading
		}
	}

	var fileHandleForWriting: FileHandle {
		if isTTY {
			return pty!
		} else {
			return stdout!.fileHandleForWriting
		}
	}

	init(tty: Bool) {
		self.isTTY = tty

		if self.isTTY {
			let master = Self.createPTY()

			self.ptx = FileHandle(fileDescriptor: master.0, closeOnDealloc: true)
			self.pty = FileHandle(fileDescriptor: master.1, closeOnDealloc: true)
			self.stdin = nil
			self.stdout = nil
		} else {
			self.ptx = nil
			self.pty = nil
			self.stdin = Pipe()
			self.stdout = Pipe()
		}
	}

	func setTermSize(rows: Int32, cols: Int32) {
		if isTTY {
			pty!.setTermSize(rows: rows, cols: cols)
		}
	}

	func eof() {
		if let ptx = self.ptx {
			ptx.closeFile()
		} else if let stdin = self.stdin {
			stdin.fileHandleForWriting.closeFile()
			stdin.fileHandleForReading.closeFile()
		}
	}

	func close() {
		if isTTY {
			self.ptx!.closeFile()
			self.pty!.closeFile()
		} else {
			self.stdin?.fileHandleForReading.closeFile()
			self.stdin?.fileHandleForWriting.closeFile()
			self.stdout?.fileHandleForReading.closeFile()
			self.stdout?.fileHandleForWriting.closeFile()
		}
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

	private static func setupty(_ fd: Int32) -> Int32 {
		let TTY_CTRL_OPTS: tcflag_t = tcflag_t(CS8 | CLOCAL | CREAD)
		let TTY_INPUT_OPTS: tcflag_t = tcflag_t(IGNPAR)
		let TTY_OUTPUT_OPTS:tcflag_t = 0
		let TTY_LOCAL_OPTS:tcflag_t = 0

		var termios_ = termios()
		var res = fcntl(fd, F_GETFL)
		if (res < 0) {
			perror("fcntl F_GETFL error")
			return res
		}

		// set serial nonblocking
		res = fcntl(fd, F_SETFL, res | O_NONBLOCK)
		if (res < 0) {
			perror("fcntl F_SETFL O_NONBLOCK error")
			return res
		}

		// set baudrate to 115200
		tcgetattr(fd, &termios_)
		cfsetispeed(&termios_, speed_t(B115200))
		cfsetospeed(&termios_, speed_t(B115200))
		cfmakeraw(&termios_)

		// This attempts to replicate the behaviour documented for cfmakeraw in
		// the termios(3) manpage.
		termios_.c_iflag = TTY_INPUT_OPTS
		termios_.c_oflag = TTY_OUTPUT_OPTS
		termios_.c_lflag = TTY_LOCAL_OPTS
		termios_.c_cflag = TTY_CTRL_OPTS
		termios_.c_cc.16 = 1 // Darwin.VMIN
		termios_.c_cc.17 = 0 // Darwin.VTIME

		if (tcsetattr(fd, TCSANOW, &termios_) != 0) {
			perror("tcsetattr error")
			return -1
		}

		return 0
	}

	private static func createPTY() -> (Int32, Int32) {
		var tty_fd: Int32 = -1
		var sfd: Int32 = -1
		let tty_path = UnsafeMutablePointer<CChar>.allocate(capacity: 1024)

		defer {
			tty_path.deallocate()
		}

		let res = openpty(&tty_fd, &sfd, tty_path, nil, nil);

		if (res < 0) {
			perror("openpty error")
			return (-1, -1)
		}

		return (tty_fd, sfd)
	}
}

