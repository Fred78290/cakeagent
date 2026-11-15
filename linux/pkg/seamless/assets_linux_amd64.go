//go:build linux && amd64

package seamless

import (
	"embed"
	"io/fs"
)

//go:embed assets/amd64
var assets embed.FS

func embeddedBinary(goarch string) []byte {
	data, _ := assets.ReadFile("assets/amd64/" + BinaryName)
	return data
}

// embeddedXKB is the XKB data tree that goes with the compositor, nil when the agent was built without it.
func embeddedXKB() fs.FS {
	sub, err := fs.Sub(assets, "assets/amd64/xkb")

	if err != nil {
		return nil
	}

	// fs.Sub does not check the directory is there: it is not when the agent was built without the data.
	if _, err = fs.Stat(sub, "rules/evdev"); err != nil {
		return nil
	}

	return sub
}
