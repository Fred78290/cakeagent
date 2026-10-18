package resize

import "errors"

func resizeDisk() error {
	return errors.New("disk resizing is not supported on windows")
}
