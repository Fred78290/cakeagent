package seamless

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// App is a launchable application described by a .desktop file.
type App struct {
	// ID is the desktop file name, "org.gnome.gedit.desktop".
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Comment    string   `json:"comment,omitempty"`
	Exec       string   `json:"exec"`
	Icon       string   `json:"icon,omitempty"`
	Terminal   bool     `json:"terminal,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

// ParseDesktopEntry reads one .desktop file. It returns nil for entries that are not applications meant to be
// shown (other types, NoDisplay, Hidden, no Exec).
func ParseDesktopEntry(id string, r io.Reader) *App {
	app := &App{ID: id}
	inEntry := false
	hidden := false
	kind := ""
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}

		if !inEntry {
			continue
		}

		key, value, ok := strings.Cut(line, "=")

		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "Type":
			kind = value
		case "Name":
			app.Name = value
		case "Comment":
			app.Comment = value
		case "Exec":
			app.Exec = value
		case "Icon":
			app.Icon = value
		case "Terminal":
			app.Terminal = value == "true"
		case "NoDisplay", "Hidden":
			if value == "true" {
				hidden = true
			}
		case "Categories":
			for _, c := range strings.Split(value, ";") {
				if c != "" {
					app.Categories = append(app.Categories, c)
				}
			}
		}
	}

	if hidden || kind != "Application" || app.Exec == "" || app.Name == "" {
		return nil
	}

	// The string-value escapes are undone first, the Exec quoting rules (applied by SplitExec) come second.
	app.Exec = StripFieldCodes(UnescapeString(app.Exec))

	return app
}

// UnescapeString undoes the escape sequences of a Desktop Entry string value: \s (space), \n, \t, \r and \\.
// Any other backslash sequence is kept untouched. This is applied before the Exec quoting rules, so a quote written
// as \\" in the file reaches SplitExec as \" (an escaped quote) and a literal backslash in a quoted argument
// takes the four backslashes the specification asks for.
func UnescapeString(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}

	var out strings.Builder

	for i := 0; i < len(value); i++ {
		c := value[i]

		if c != '\\' || i+1 >= len(value) {
			out.WriteByte(c)
			continue
		}

		switch value[i+1] {
		case 's':
			out.WriteByte(' ')
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case '\\':
			out.WriteByte('\\')
		default:
			out.WriteByte(c)
			continue
		}

		i++
	}

	return out.String()
}

// StripFieldCodes removes the %f/%U style placeholders of an Exec line, we launch without arguments.
// Whitespace outside double quotes is collapsed to single spaces, whitespace inside quoted arguments
// (including quoted executable paths) is preserved as is.
func StripFieldCodes(exec string) string {
	var out strings.Builder

	inQuote := false
	pendingSpace := false

	write := func(c byte) {
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}

		out.WriteByte(c)
	}

	for i := 0; i < len(exec); i++ {
		c := exec[i]

		switch {
		case c == '%' && i+1 < len(exec):
			i++

			if exec[i] == '%' {
				write('%')
			}
		case c == '\\' && i+1 < len(exec):
			write(c)
			i++
			write(exec[i])
		case c == '"':
			inQuote = !inQuote
			write(c)
		case !inQuote && (c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'):
			pendingSpace = out.Len() > 0
		default:
			write(c)
		}
	}

	return out.String()
}

// SplitExec decodes an Exec line into an argument vector following the Desktop Entry specification: arguments are
// separated by whitespace, a double-quoted argument may contain whitespace, and inside quotes a backslash escapes
// the next character (\" \` \$ \\), so an escaped dollar sign stays a literal one. No shell is involved: nothing
// is expanded. An unterminated quote is an error.
func SplitExec(exec string) ([]string, error) {
	var (
		args    []string
		current strings.Builder
	)

	inQuote, started := false, false

	flush := func() {
		if started {
			args = append(args, current.String())
			current.Reset()

			started = false
		}
	}

	for i := 0; i < len(exec); i++ {
		c := exec[i]

		switch {
		case c == '\\' && i+1 < len(exec):
			i++
			current.WriteByte(exec[i])

			started = true
		case c == '"':
			inQuote = !inQuote
			started = true
		case !inQuote && (c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'):
			flush()
		default:
			current.WriteByte(c)

			started = true
		}
	}

	if inQuote {
		return nil, fmt.Errorf("unterminated quote in Exec line %q", exec)
	}

	flush()

	return args, nil
}

// ApplicationDirs are the directories .desktop files are looked up in, for a user's home.
func ApplicationDirs(home string) []string {
	dataDirs := os.Getenv("XDG_DATA_DIRS")

	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}

	var dirs []string

	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local/share/applications"),
			// Per-user Flatpak installs export their desktop files here.
			filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		)
	}

	for _, d := range strings.Split(dataDirs, ":") {
		if d != "" {
			dirs = append(dirs, filepath.Join(d, "applications"))
		}
	}

	return append(dirs, "/var/lib/flatpak/exports/share/applications", "/var/lib/snapd/desktop/applications")
}

// ListApps reads the .desktop files of dirs (the first directory that has an ID wins) sorted by name.
func ListApps(dirs []string) []App {
	seen := map[string]bool{}
	var apps []App

	for _, dir := range dirs {
		// WalkDir does not follow a symlinked root (it would see a single non-directory entry), resolve it first.
		// IDs are computed against the resolved root, the one actually traversed.
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			dir = resolved
		}

		// Desktop files may live in subdirectories, "vendor/editor.desktop" has the ID "vendor-editor.desktop".
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				// Skip unreadable directories, keep going with the others.
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}

				return nil
			}

			if entry.IsDir() || !strings.HasSuffix(path, ".desktop") {
				return nil
			}

			rel, err := filepath.Rel(dir, path)

			if err != nil {
				return nil
			}

			id := strings.ReplaceAll(rel, string(filepath.Separator), "-")

			if seen[id] {
				return nil
			}

			file, err := os.Open(path)

			if err != nil {
				return nil
			}

			app := ParseDesktopEntry(id, file)

			file.Close()

			// An ID that is hidden in a higher priority directory stays hidden.
			seen[id] = true

			if app != nil {
				apps = append(apps, *app)
			}

			return nil
		})
	}

	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name) })

	return apps
}

// FindApp looks an application up by desktop file ID (with or without ".desktop").
func FindApp(apps []App, id string) (App, bool) {
	if !strings.HasSuffix(id, ".desktop") {
		id += ".desktop"
	}

	for _, a := range apps {
		if a.ID == id {
			return a, true
		}
	}

	return App{}, false
}

// ParseLang extracts LANG from a locale configuration file ("LANG=en_US.UTF-8", optionally quoted).
func ParseLang(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)

		if value, ok := strings.CutPrefix(line, "LANG="); ok {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}

	return ""
}
