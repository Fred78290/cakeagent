package seamless

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
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

func TestPickAccountRejectsUIDZero(t *testing.T) {
	accounts := ParsePasswd(strings.NewReader("root:x:0:0::/root:/bin/bash\ntoor:x:0:0::/root:/bin/bash\nalice:x:1000:1000::/home/alice:/bin/zsh\n"))

	for _, name := range []string{"root", "toor"} {
		if a, err := PickAccount(accounts, name); err == nil {
			t.Fatalf("expected %q to be rejected, got %+v", name, a)
		}
	}

	if a, err := PickAccount(accounts, "alice"); err != nil || a.UID != 1000 {
		t.Fatalf("got %+v, %v", a, err)
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

func TestSplitExec(t *testing.T) {
	cases := map[string][]string{
		"gedit":                               {"gedit"},
		"firefox --new-window":                {"firefox", "--new-window"},
		`"/opt/My App/run" --x "a  b"`:        {"/opt/My App/run", "--x", "a  b"},
		`app "cost: \$5"`:                     {"app", "cost: $5"},
		"app \"say \\\"hi\\\" \\\\ \\`x\\`\"": {"app", "say \"hi\" \\ `x`"},
		`app ""`:                              {"app", ""},
		`sh -c "echo \$HOME"`:                 {"sh", "-c", "echo $HOME"},
	}

	for in, want := range cases {
		got, err := SplitExec(in)

		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("SplitExec(%q) = %q, %v, want %q", in, got, err, want)
		}
	}

	if _, err := SplitExec(`app "oops`); err == nil {
		t.Error("expected an error for an unterminated quote")
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

func TestParseDesktopEntryUnescapesExecBeforeSplitting(t *testing.T) {
	cases := map[string][]string{
		// Encoded inner quotes (\\" in the file) stay inside the quoted argument, the path with a space is one argument.
		`"/opt/My App/run" "say \\"hi\\" %f"`: {"/opt/My App/run", `say "hi" `},
		// Four backslashes are a literal backslash in a quoted argument.
		`app "a\\\\b"`: {"app", `a\b`},
		// \s is a space, even outside quotes it is a separator once decoded.
		`app\sflag`: {"app", "flag"},
		// Unknown sequences are left alone.
		`app "cost: \$5"`: {"app", "cost: $5"},
	}

	for exec, want := range cases {
		app := ParseDesktopEntry("x.desktop", strings.NewReader("[Desktop Entry]\nType=Application\nName=X\nExec="+exec+"\n"))

		if app == nil {
			t.Fatalf("Exec=%s was skipped", exec)
		}

		got, err := SplitExec(app.Exec)

		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Exec=%s: SplitExec(%q) = %q, %v, want %q", exec, app.Exec, got, err, want)
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

func TestListAppsTraversesSubdirectories(t *testing.T) {
	high, low := t.TempDir(), t.TempDir()

	write := func(dir, name, body string) {
		path := filepath.Join(dir, name)

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(high, "vendor/editor.desktop", "[Desktop Entry]\nType=Application\nName=Editor (user)\nExec=editor-user\n")
	write(low, "vendor/editor.desktop", "[Desktop Entry]\nType=Application\nName=Editor (system)\nExec=editor-system\n")
	write(low, "vendor/deep/tool.desktop", "[Desktop Entry]\nType=Application\nName=Tool\nExec=tool\n")

	apps := ListApps([]string{high, low})

	if len(apps) != 2 {
		t.Fatalf("unexpected %+v", apps)
	}

	if app, ok := FindApp(apps, "vendor-editor"); !ok || app.Exec != "editor-user" {
		t.Fatalf("vendor-editor: %+v %v", app, ok)
	}

	if app, ok := FindApp(apps, "vendor-deep-tool"); !ok || app.Exec != "tool" {
		t.Fatalf("vendor-deep-tool: %+v %v", app, ok)
	}
}

func TestListAppsFollowsSymlinkedRoot(t *testing.T) {
	real := t.TempDir()

	if err := os.MkdirAll(filepath.Join(real, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}

	body := "[Desktop Entry]\nType=Application\nName=Editor\nExec=editor\n"

	if err := os.WriteFile(filepath.Join(real, "vendor", "editor.desktop"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "applications")

	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	apps := ListApps([]string{link})

	if len(apps) != 1 {
		t.Fatalf("unexpected %+v", apps)
	}

	if _, ok := FindApp(apps, "vendor-editor"); !ok {
		t.Fatalf("the ID must be relative to the resolved root: %+v", apps)
	}
}

func TestWriteIfChangedIsAtomicAndSkipsIdenticalContent(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "libexec", "caker", BinaryName)

	if changed, err := writeIfChanged(dest, []byte("v1"), 0o755); err != nil || !changed {
		t.Fatalf("first write: %v %v", changed, err)
	}

	if changed, err := writeIfChanged(dest, []byte("v1"), 0o755); err != nil || changed {
		t.Fatalf("identical write: %v %v", changed, err)
	}

	if changed, err := writeIfChanged(dest, []byte("v2"), 0o755); err != nil || !changed {
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

	if changed, err := writeIfChanged(dest, []byte("v2"), 0o755); err != nil || !changed {
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

func TestInstallXKBCopiesTheTreeReadableByEveryone(t *testing.T) {
	tree := fstest.MapFS{
		"rules/evdev":    {Data: []byte("rules")},
		"symbols/us":     {Data: []byte("us")},
		"symbols/pc":     {Data: []byte("pc")},
		"keycodes/evdev": {Data: []byte("keycodes")},
	}
	dir := filepath.Join(t.TempDir(), "xkb")

	if changed, err := installXKB(tree, dir); err != nil || !changed {
		t.Fatalf("first install: %v %v", changed, err)
	}

	for name, entry := range tree {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)

		if err != nil || !bytes.Equal(data, entry.Data) {
			t.Fatalf("%s: %q %v", name, data, err)
		}

		if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
			t.Fatalf("%s: mode %v", name, info.Mode().Perm())
		}

		if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o755 {
			t.Fatalf("%s: directory mode %v", name, info.Mode().Perm())
		}
	}

	if changed, err := installXKB(tree, dir); err != nil || changed {
		t.Fatalf("second install: %v %v", changed, err)
	}

	if changed, err := installXKB(nil, dir); err != nil || changed {
		t.Fatalf("no tree: %v %v", changed, err)
	}
}

func TestInstallXKBRetriesAnIncompleteCopy(t *testing.T) {
	tree := fstest.MapFS{
		"rules/evdev": {Data: []byte("rules")},
		"symbols/us":  {Data: []byte("us")},
	}
	binary := filepath.Join(t.TempDir(), BinaryName)
	dir := XKBDir(binary)

	if XKBInstalled(binary) {
		t.Fatal("nothing installed yet")
	}

	if _, err := installXKB(tree, dir); err != nil || !XKBInstalled(binary) {
		t.Fatalf("install: %v, complete %v", err, XKBInstalled(binary))
	}

	// A copy interrupted after the sentinel file was written, before the rest.
	if err := os.Remove(filepath.Join(dir, xkbComplete)); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(dir, "symbols", "us")); err != nil {
		t.Fatal(err)
	}

	if !hasXKBDataForTest(dir) || XKBInstalled(binary) {
		t.Fatal("the sentinel file alone must not count as installed")
	}

	if changed, err := installXKB(tree, dir); err != nil || !changed || !XKBInstalled(binary) {
		t.Fatalf("retry: %v %v", changed, err)
	}

	if _, err := os.Stat(filepath.Join(dir, "symbols", "us")); err != nil {
		t.Fatalf("the missing file was not restored: %v", err)
	}
}

func hasXKBDataForTest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "rules", "evdev"))

	return err == nil
}

func TestXKBDirIsBesideTheBinary(t *testing.T) {
	if got := XKBDir("/usr/libexec/caker/caker-seamlessd"); got != "/usr/libexec/caker/xkb" {
		t.Fatalf("XKBDir: %s", got)
	}
}
