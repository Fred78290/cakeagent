package seamless

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionOfReadsTheMarker(t *testing.T) {
	binary := append([]byte("\x7fELF....some code...."), []byte("caker-seamlessd-version:v1.0.335\x00more data caker-seamlessd v1.0.335\x00")...)

	if got := VersionOf(binary); got != "v1.0.335" {
		t.Fatalf("VersionOf = %q", got)
	}
}

func TestVersionOfDevBuild(t *testing.T) {
	if got := VersionOf([]byte("xx caker-seamlessd-version:dev\x00")); got != "dev" {
		t.Fatalf("VersionOf = %q", got)
	}
}

func TestVersionOfStopsAtTheEndOfTheString(t *testing.T) {
	// No terminator but a space or a newline: only the version is taken.
	if got := VersionOf([]byte("caker-seamlessd-version:v1.0.1 and the rest")); got != "v1.0.1" {
		t.Fatalf("VersionOf = %q", got)
	}

	// Never more than a bounded length of a corrupt binary.
	long := append([]byte("caker-seamlessd-version:"), make([]byte, 200)...)

	for i := range long[len("caker-seamlessd-version:"):] {
		long[len("caker-seamlessd-version:")+i] = 'a'
	}

	if got := VersionOf(long); len(got) != 64 {
		t.Fatalf("VersionOf length = %d, want 64", len(got))
	}
}

func TestVersionOfOldCompositorWithoutTheMarker(t *testing.T) {
	if got := VersionOf([]byte("...caker-seamlessd 0.1...")); got != UnknownVersion+" (before the version marker)" {
		t.Fatalf("VersionOf = %q", got)
	}

	if got := VersionOf([]byte("not a compositor")); got != UnknownVersion {
		t.Fatalf("VersionOf = %q", got)
	}

	// An empty value after the marker is no version.
	if got := VersionOf([]byte("caker-seamlessd-version:\x00")); got != UnknownVersion {
		t.Fatalf("VersionOf = %q", got)
	}
}

func TestInstalledVersionReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), BinaryName)

	if err := os.WriteFile(path, []byte("x caker-seamlessd-version:v1.0.7\x00"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := InstalledVersion(path)

	if err != nil || got != "v1.0.7" {
		t.Fatalf("InstalledVersion = %q, %v", got, err)
	}

	if _, err := InstalledVersion(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing file must be an error")
	}
}
