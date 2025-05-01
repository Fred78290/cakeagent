import Foundation
import NIOPosix
import System

public extension FileHandle {
	func getTermSize() -> (rows: Int32, cols: Int32) {
		var rows: UInt16 = 0
		var cols: UInt16 = 0

		if self.isTTY() {
			var size = winsize()

			if ioctl(self.fileDescriptor, TIOCGWINSZ, &size) == 0 {
				rows = size.ws_row
				cols = size.ws_col
			}
		}

		return (Int32(rows), Int32(cols))
	}

	func closeOnExec() throws {
		if fcntl(self.fileDescriptor, F_SETFD, FD_CLOEXEC) < 0 {
			throw Errno(rawValue: errno)
		}
	}

	func setNonblock() throws {
		if fcntl(self.fileDescriptor, F_SETFL, O_NONBLOCK) < 0 {
			throw Errno(rawValue: errno)
		}
	}

	func setTermSize(rows: Int32, cols: Int32) throws {
		if self.isTTY() {
			var size = winsize()

			size.ws_row = UInt16(rows)
			size.ws_col = UInt16(cols)

			if ioctl(self.fileDescriptor, TIOCSWINSZ, &size) != 0 {
				throw Errno(rawValue: errno)
			}
		}
	}

	func isTTY() -> Bool {
		return isatty(self.fileDescriptor) != 0
	}

	func getState() throws -> termios {
		var term: termios = termios()

		if self.isTTY() {
			if tcgetattr(self.fileDescriptor, &term) != 0 {
				throw Errno(rawValue: errno)
			}
		}

		return term
	}

	func makeRaw() throws -> termios {
		var term: termios = termios()

		if self.isTTY() {
			if tcgetattr(self.fileDescriptor, &term) != 0 {
				throw Errno(rawValue: errno)
			}

			var newState: termios = term

			newState.c_iflag &= UInt(IGNBRK) | ~UInt(BRKINT | INPCK | ISTRIP | IXON)
			newState.c_cflag |= UInt(CS8)
			newState.c_lflag &= ~UInt(ECHO | ICANON | IEXTEN | ISIG)
			newState.c_cc.16 = 1
			newState.c_cc.17 = 17

			if tcsetattr(self.fileDescriptor, TCSANOW, &newState) != 0 {
				throw Errno(rawValue: errno)
			}
		}

		return term
	}

	func cfmakeRaw() throws -> termios {
		var term: termios = termios()

		if self.isTTY(){
			if tcgetattr(self.fileDescriptor, &term) != 0 {
				throw Errno(rawValue: errno)
			}

			var newState: termios = term

			cfmakeraw(&newState)

			if tcsetattr(self.fileDescriptor, TCSANOW, &newState) != 0 {
				throw Errno(rawValue: errno)
			}
		}

		return term
	}

	func restoreState(_ term: UnsafePointer<termios>) throws {
		if self.isTTY() {
			if tcsetattr(self.fileDescriptor, TCSANOW, term) != 0 {
				throw Errno(rawValue: errno)
			}
		}
	}

	func fileDescriptorIsFile() throws -> Bool {
        var s: stat = .init()
		
		if Darwin.fstat(self.fileDescriptor, &s) != 0 {
			throw Errno(rawValue: errno)
		}

        switch s.st_mode & S_IFMT {
        case S_IFREG, S_IFDIR, S_IFLNK, S_IFBLK:
            return true
        default:
            return false
        }
	}
}
