//go:build linux && amd64

package seamless

import "embed"

//go:embed assets/amd64
var assets embed.FS

func embeddedBinary(goarch string) []byte {
	data, _ := assets.ReadFile("assets/amd64/" + BinaryName)
	return data
}
