package seamless

import (
	"fmt"
	"strings"
)

// DefaultProfilePath is the shell profile snippet that lets a plain command typed in the guest (ssh, console,
// "caked sh", a terminal opened on the desktop) open its window on the compositor, without "seamless launch".
const DefaultProfilePath = "/etc/profile.d/caker-seamless.sh"

// SessionProfile returns the shell snippet. It is conditional on purpose: it does nothing while the compositor's
// socket does not exist (seamless windows turned off, agent stopped) or when a display is already set (the guest
// has a desktop session of its own), so it can stay installed. The backends are lists, so that an application
// without Wayland support falls back to X11 instead of failing.
func SessionProfile(cfg Config) string {
	cfg = cfg.WithDefaults()

	// The socket name goes into a shell word: it is never trusted to be plain.
	name := shellQuote(cfg.SocketName)

	return fmt.Sprintf(`# Installed by cakeagent for Caker's seamless windows. Applications started from a shell show up on the host.
if [ -z "${WAYLAND_DISPLAY:-}" ]; then
	_caker_rt="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

	if [ -S "$_caker_rt/"%[1]s ]; then
		export XDG_RUNTIME_DIR="$_caker_rt"
		export WAYLAND_DISPLAY=%[1]s
		export XDG_SESSION_TYPE=wayland
		export GDK_BACKEND="wayland,x11"
		export QT_QPA_PLATFORM="wayland;xcb"

		# X11 applications: the compositor serves an X display (Xwayland) and writes its name next to its socket.
		if [ -z "${DISPLAY:-}" ] && [ -r "$_caker_rt/"%[1]s.display ]; then
			_caker_x11="$(head -n 1 "$_caker_rt/"%[1]s.display 2>/dev/null)"

			case "$_caker_x11" in
				:[0-9]*) export DISPLAY="$_caker_x11" ;;
			esac

			unset _caker_x11
		fi

		export MOZ_ENABLE_WAYLAND=1
		export ELECTRON_OZONE_PLATFORM_HINT=auto

		if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ] && [ -S "$_caker_rt/bus" ]; then
			export DBUS_SESSION_BUS_ADDRESS="unix:path=$_caker_rt/bus"
		fi
	fi

	unset _caker_rt
fi
`, name)
}

// shellQuote quotes s as one POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// InstallSessionProfile writes the profile snippet (DefaultProfilePath unless cfg.ProfilePath is set) and reports
// whether it changed.
func InstallSessionProfile(cfg Config) (bool, error) {
	cfg = cfg.WithDefaults()

	return writeIfChanged(cfg.ProfilePath, []byte(SessionProfile(cfg)), 0o644)
}
