//go:build linux && arm64

package seamless

import "embed"

//go:embed assets/arm64
var assets embed.FS

func embeddedBinary(goarch string) []byte {
	data, _ := assets.ReadFile("assets/arm64/" + BinaryName)
	return data
}
