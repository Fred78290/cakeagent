package seamless

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnvBinary points Install at a compositor binary when none is embedded (development builds of the agent).
const EnvBinary = "CAKER_SEAMLESSD"

// sourceBinary returns the compositor to install: the one embedded in the agent, else $CAKER_SEAMLESSD, else one
// found on PATH.
func sourceBinary() ([]byte, string, error) {
	if data := embeddedBinary(runtime.GOARCH); len(data) > 0 {
		return data, "embedded", nil
	}

	candidates := []string{os.Getenv(EnvBinary)}

	if path, err := exec.LookPath(BinaryName); err == nil {
		candidates = append(candidates, path)
	}

	for _, path := range candidates {
		if path == "" {
			continue
		}

		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data, path, nil
		}
	}

	return nil, "", fmt.Errorf("no %s binary: this agent was built without one (see linux/pkg/seamless/assets), set %s or put it on PATH", BinaryName, EnvBinary)
}

// Install writes the compositor to dest (DefaultBinary when empty) unless it is already identical. It returns
// where the binary came from and whether the file changed.
func Install(dest string) (source string, changed bool, err error) {
	if dest == "" {
		dest = DefaultBinary
	}

	data, source, err := sourceBinary()

	if err != nil {
		return "", false, err
	}

	changed, err = writeIfChanged(dest, data)

	return source, changed, err
}

func writeIfChanged(dest string, data []byte) (bool, error) {
	if current, err := os.ReadFile(dest); err == nil && sha256.Sum256(current) == sha256.Sum256(data) {
		if info, err := os.Stat(dest); err == nil && info.Mode().Perm() == 0o755 {
			return false, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}

	// A running compositor keeps its old inode: write next to it and rename, never into the file in use.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".caker-seamlessd-*")

	if err != nil {
		return false, err
	}

	defer os.Remove(tmp.Name())

	if _, err = bytes.NewReader(data).WriteTo(tmp); err == nil {
		err = tmp.Chmod(0o755)
	}

	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return false, err
	}

	return true, os.Rename(tmp.Name(), dest)
}
