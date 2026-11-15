package seamless

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const passwd = `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
# comment
malformed line
admin:x:1001:1001:Admin:/home/admin:/bin/bash
alice:x:1000:1000:Alice:/home/alice:/bin/zsh
svc:x:1002:1002::/var/svc:/bin/false
nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin
`

func TestParsePasswdSkipsCommentsAndMalformedLines(t *testing.T) {
	accounts := ParsePasswd(strings.NewReader(passwd))

	if len(accounts) != 6 {
		t.Fatalf("got %d accounts: %+v", len(accounts), accounts)
	}

	if accounts[2].Name != "admin" || accounts[2].UID != 1001 || accounts[2].Home != "/home/admin" || accounts[2].Shell != "/bin/bash" {
		t.Fatalf("unexpected account %+v", accounts[2])
	}
}

func TestPickAccountDefaultsToTheFirstRegularUser(t *testing.T) {
	accounts := ParsePasswd(strings.NewReader(passwd))

	a, err := PickAccount(accounts, "")

	if err != nil || a.Name != "alice" {
		t.Fatalf("got %+v, %v", a, err)
	}
}

func TestPickAccountByNameAndMissing(t *testing.T) {
	accounts := ParsePasswd(strings.NewReader(passwd))

	if a, err := PickAccount(accounts, "admin"); err != nil || a.UID != 1001 {
		t.Fatalf("got %+v, %v", a, err)
	}

	if _, err := PickAccount(accounts, "bob"); err == nil {
		t.Fatal("expected an error for an unknown user")
	}

	// System accounts are never picked by default.
	if _, err := PickAccount(ParsePasswd(strings.NewReader("root:x:0:0::/root:/bin/bash\nsvc:x:999:999::/:/bin/false\n")), ""); err == nil {
		t.Fatal("expected no regular user")
	}
}

func TestStripFieldCodes(t *testing.T) {
	cases := map[string]string{
		"gedit %U":                     "gedit",
		"firefox --new-window %u":      "firefox --new-window",
		`sh -c "echo 100%%" %f`:        `sh -c "echo 100%"`,
		"app":                          "app",
		"app   %F   --flag   %i %c %k": "app --flag",
	}

	for in, want := range cases {
		if got := StripFieldCodes(in); got != want {
			t.Errorf("StripFieldCodes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDesktopEntry(t *testing.T) {
	entry := `[Desktop Entry]
Type=Application
Name=Text Editor
Name[fr]=Éditeur de texte
Comment=Edit text
Exec=gedit %U
Icon=org.gnome.gedit
Categories=Utility;TextEditor;

[Desktop Action new-window]
Name=New Window
Exec=gedit --new-window
`

	app := ParseDesktopEntry("org.gnome.gedit.desktop", strings.NewReader(entry))

	if app == nil || app.Name != "Text Editor" || app.Exec != "gedit" || app.Icon != "org.gnome.gedit" || len(app.Categories) != 2 {
		t.Fatalf("unexpected %+v", app)
	}

	for _, hidden := range []string{
		"[Desktop Entry]\nType=Application\nName=X\nExec=x\nNoDisplay=true\n",
		"[Desktop Entry]\nType=Application\nName=X\nExec=x\nHidden=true\n",
		"[Desktop Entry]\nType=Link\nName=X\nURL=http://x\n",
		"[Desktop Entry]\nType=Application\nName=X\n",
	} {
		if ParseDesktopEntry("x.desktop", strings.NewReader(hidden)) != nil {
			t.Errorf("expected %q to be skipped", hidden)
		}
	}
}

func TestListAppsFirstDirectoryWinsAndSortsByName(t *testing.T) {
	high, low := t.TempDir(), t.TempDir()

	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(high, "b.desktop", "[Desktop Entry]\nType=Application\nName=Bravo (user)\nExec=b-user\n")
	write(low, "b.desktop", "[Desktop Entry]\nType=Application\nName=Bravo (system)\nExec=b-system\n")
	write(low, "a.desktop", "[Desktop Entry]\nType=Application\nName=alpha\nExec=a\n")
	write(low, "hidden.desktop", "[Desktop Entry]\nType=Application\nName=H\nExec=h\n")
	write(high, "hidden.desktop", "[Desktop Entry]\nType=Application\nName=H\nExec=h\nNoDisplay=true\n")
	write(low, "readme.txt", "not a desktop file")

	apps := ListApps([]string{high, low})

	if len(apps) != 2 || apps[0].Name != "alpha" || apps[1].Exec != "b-user" {
		t.Fatalf("unexpected %+v", apps)
	}

	if app, ok := FindApp(apps, "b"); !ok || app.ID != "b.desktop" {
		t.Fatalf("FindApp: %+v %v", app, ok)
	}

	if _, ok := FindApp(apps, "hidden"); ok {
		t.Fatal("a hidden entry in a higher priority directory must stay hidden")
	}
}

func TestWriteIfChangedIsAtomicAndSkipsIdenticalContent(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "libexec", "caker", BinaryName)

	if changed, err := writeIfChanged(dest, []byte("v1")); err != nil || !changed {
		t.Fatalf("first write: %v %v", changed, err)
	}

	if changed, err := writeIfChanged(dest, []byte("v1")); err != nil || changed {
		t.Fatalf("identical write: %v %v", changed, err)
	}

	if changed, err := writeIfChanged(dest, []byte("v2")); err != nil || !changed {
		t.Fatalf("update: %v %v", changed, err)
	}

	data, _ := os.ReadFile(dest)
	info, _ := os.Stat(dest)

	if string(data) != "v2" || info.Mode().Perm() != 0o755 {
		t.Fatalf("got %q, mode %v", data, info.Mode())
	}

	entries, _ := os.ReadDir(filepath.Dir(dest))

	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}

	// A file with the right content but the wrong mode is fixed.
	if err := os.Chmod(dest, 0o600); err != nil {
		t.Fatal(err)
	}

	if changed, err := writeIfChanged(dest, []byte("v2")); err != nil || !changed {
		t.Fatalf("mode fix: %v %v", changed, err)
	}
}

func TestInstallFallsBackToTheEnvironmentOverride(t *testing.T) {
	src := filepath.Join(t.TempDir(), "compositor")
	dest := filepath.Join(t.TempDir(), "out", BinaryName)

	if err := os.WriteFile(src, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvBinary, src)
	t.Setenv("PATH", t.TempDir())

	source, changed, err := Install(dest)

	// A build with an embedded compositor installs that one instead: both are fine here.
	if err != nil || !changed {
		t.Fatalf("Install: %v %v %v", source, changed, err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}.WithDefaults()

	if c.Port != 5100 || c.SocketName != "caker-seamless" || c.Binary != DefaultBinary || c.Connect() != "vsock:5100" {
		t.Fatalf("unexpected %+v", c)
	}

	if (Config{Port: 6000}).Connect() != "vsock:6000" {
		t.Fatal("custom port ignored")
	}
}

func TestParseLang(t *testing.T) {
	cases := map[string]string{
		"LANG=en_US.UTF-8\n":                             "en_US.UTF-8",
		"# comment\nLANGUAGE=en\nLANG=\"fr_FR.UTF-8\"\n": "fr_FR.UTF-8",
		"LC_ALL=C\n": "",
		"":           "",
	}

	for in, want := range cases {
		if got := ParseLang(in); got != want {
			t.Errorf("ParseLang(%q) = %q, want %q", in, got, want)
		}
	}
}
