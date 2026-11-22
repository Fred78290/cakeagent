package seamless

import (
	"bytes"
	"os"
	"runtime"
)

// versionMarker precedes the version string compiled into every caker-seamlessd (seamlessd/main.c,
// SEAMLESSD_VERSION_MARKER): it is read straight out of the binary, so the version of the compositor embedded in the
// agent, of the one installed and of the ones running can be told without starting any of them.
const versionMarker = "caker-seamlessd-version:"

// UnknownVersion is reported for a compositor built before the version marker existed.
const UnknownVersion = "unknown"

// legacyIdentity is what a compositor without the marker called itself in its hello to the host.
var legacyIdentity = []byte("caker-seamlessd 0.1")

// VersionOf reads the version out of the bytes of a compositor: "dev" for a build made by hand, the release of the agent
// (v1.0.<commit count>) for one built by CI, UnknownVersion for a compositor that predates the marker.
func VersionOf(binary []byte) string {
	if i := bytes.Index(binary, []byte(versionMarker)); i >= 0 {
		rest := binary[i+len(versionMarker):]
		n := 0

		for n < len(rest) && n < 64 && rest[n] > ' ' && rest[n] < 0x7f {
			n++
		}

		if n > 0 {
			return string(rest[:n])
		}
	}

	if bytes.Contains(binary, legacyIdentity) {
		return UnknownVersion + " (before the version marker)"
	}

	return UnknownVersion
}

// EmbeddedVersion is the version of the compositor this agent carries, false when it was built without one (a development
// build of the agent).
func EmbeddedVersion() (string, bool) {
	data := embeddedBinary(runtime.GOARCH)

	if len(data) == 0 {
		return "", false
	}

	return VersionOf(data), true
}

// InstalledVersion is the version of the compositor at path.
func InstalledVersion(path string) (string, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return "", err
	}

	return VersionOf(data), nil
}
