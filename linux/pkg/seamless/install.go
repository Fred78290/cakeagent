package seamless

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
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

// XKBDir is where the XKB data of the compositor installed at binary lives: beside it.
func XKBDir(binary string) string {
	return filepath.Join(filepath.Dir(binary), "xkb")
}

// xkbComplete is the file written last into the XKB directory: its presence means the whole tree was copied.
const xkbComplete = ".caker-complete"

// XKBInstalled tells whether the XKB data beside the compositor at binary was copied completely, which a single file
// of the tree cannot tell: the copy may have stopped half way.
func XKBInstalled(binary string) bool {
	info, err := os.Stat(filepath.Join(XKBDir(binary), xkbComplete))

	return err == nil && info.Mode().IsRegular()
}

// Install writes the compositor to dest (DefaultBinary when empty) unless it is already identical, and the XKB
// data it compiles its keymap from beside it when the agent embeds that data. It returns where the binary came
// from and whether anything changed.
func Install(dest string) (source string, changed bool, err error) {
	if dest == "" {
		dest = DefaultBinary
	}

	data, source, err := sourceBinary()

	if err != nil {
		return "", false, err
	}

	changed, err = writeIfChanged(dest, data, 0o755)

	if err != nil {
		return source, changed, err
	}

	xkbChanged, err := installXKB(embeddedXKB(), XKBDir(dest))

	return source, changed || xkbChanged, err
}

// installXKB copies the XKB data tree into dir (readable by every user: the compositor runs unprivileged). A nil
// tree installs nothing.
func installXKB(tree fs.FS, dir string) (changed bool, err error) {
	if tree == nil {
		return false, nil
	}

	marker := filepath.Join(dir, xkbComplete)

	// The tree is incomplete from here until the copy has succeeded.
	_, statErr := os.Stat(marker)
	wasComplete := statErr == nil

	if err = os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return false, err
	}

	err = fs.WalkDir(tree, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		dest := filepath.Join(dir, filepath.FromSlash(name))

		if d.IsDir() {
			if err = os.MkdirAll(dest, 0o755); err != nil {
				return err
			}

			return os.Chmod(dest, 0o755)
		}

		data, err := fs.ReadFile(tree, name)

		if err != nil {
			return err
		}

		fileChanged, err := writeIfChanged(dest, data, 0o644)
		changed = changed || fileChanged

		return err
	})

	if err != nil {
		return changed, err
	}

	// Recorded only now that every file is in place, so an interrupted copy is retried.
	if _, err = writeIfChanged(marker, []byte("ok\n"), 0o644); err != nil {
		return changed, err
	}

	return changed || !wasComplete, nil
}

func writeIfChanged(dest string, data []byte, mode os.FileMode) (bool, error) {
	if current, err := os.ReadFile(dest); err == nil && sha256.Sum256(current) == sha256.Sum256(data) {
		if info, err := os.Stat(dest); err == nil && info.Mode().Perm() == mode {
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
		err = tmp.Chmod(mode)
	}

	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return false, err
	}

	return true, os.Rename(tmp.Name(), dest)
}
