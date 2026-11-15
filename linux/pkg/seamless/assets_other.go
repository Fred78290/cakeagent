//go:build !(linux && (amd64 || arm64))

package seamless

// No compositor is embedded outside linux/amd64 and linux/arm64 (nor in other operating systems' agents).
func embeddedBinary(goarch string) []byte { return nil }
