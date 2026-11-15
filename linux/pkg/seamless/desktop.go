package seamless

import (
	"bufio"
	"io"
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

	app.Exec = StripFieldCodes(app.Exec)

	return app
}

// StripFieldCodes removes the %f/%U style placeholders of an Exec line, we launch without arguments.
func StripFieldCodes(exec string) string {
	var out strings.Builder

	for i := 0; i < len(exec); i++ {
		if exec[i] != '%' || i+1 >= len(exec) {
			out.WriteByte(exec[i])
			continue
		}

		i++

		if exec[i] == '%' {
			out.WriteByte('%')
		}
	}

	return strings.Join(strings.Fields(out.String()), " ")
}

// ApplicationDirs are the directories .desktop files are looked up in, for a user's home.
func ApplicationDirs(home string) []string {
	dataDirs := os.Getenv("XDG_DATA_DIRS")

	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}

	var dirs []string

	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local/share/applications"))
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
		entries, err := os.ReadDir(dir)

		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()

			if entry.IsDir() || !strings.HasSuffix(name, ".desktop") || seen[name] {
				continue
			}

			file, err := os.Open(filepath.Join(dir, name))

			if err != nil {
				continue
			}

			app := ParseDesktopEntry(name, file)

			file.Close()

			// An ID that is hidden in a higher priority directory stays hidden.
			seen[name] = true

			if app != nil {
				apps = append(apps, *app)
			}
		}
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
