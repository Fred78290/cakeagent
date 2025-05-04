package resize

import "errors"

func resizeDisk() error {
	// No-op on Linux, as resizing is not needed
	// and not supported by the OS.
	return errors.New("disk resizing is not supported on Linux")
}

// This function is intentionally left empty.
// The disk resizing logic is handled in the diskresizer package for macOS.
// The ResizeDisk function is a no-op on Linux, as resizing is not needed and not supported by the OS.
