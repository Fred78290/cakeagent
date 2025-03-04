import Foundation

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

	func setTermSize(rows: Int32, cols: Int32) {
		print("""
			Setting terminal size to \(rows) rows and \(cols) columns
			""")
		if self.isTTY() {
			var size = winsize()

			size.ws_row = UInt16(rows)
			size.ws_col = UInt16(cols)

			if ioctl(self.fileDescriptor, TIOCSWINSZ, &size) != 0 {
				perror("ioctl error")
			}
		}
	}

	func isTTY() -> Bool {
		return isatty(self.fileDescriptor) != 0
	}

	func makeRaw() -> termios {
		var term: termios = termios()

		if self.isTTY() {
			if tcgetattr(self.fileDescriptor, &term) != 0 {
				perror("tcgetattr error")
			}

			var newState: termios = term

			cfmakeraw(&newState)

			if tcsetattr(self.fileDescriptor, TCSANOW, &newState) != 0 {
				perror("tcsetattr error")
			}
		}

		return term
	}

	func restoreState(_ term: UnsafePointer<termios>) {
		if self.isTTY() {
			if tcsetattr(self.fileDescriptor, TCSANOW, term) != 0 {
				perror("tcsetattr error")
			}
		}
	}
}
