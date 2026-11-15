// Package seamless runs Caker's seamless windows compositor inside a Linux guest.
//
// caker-seamlessd (Tools/SeamlessCompositor in the caker repository) is a headless Wayland compositor that streams
// every window to the macOS host over vsock. This package makes the agent the thing that ships and supervises it:
//
//   - Install puts the compositor binary (embedded in the agent at build time, see assets/) at a fixed path, with
//     the XKB data it needs beside it for guests that have none.
//   - Run starts it as the guest's login user, restarts it when it dies, and stops it with the agent.
//   - Launch starts a program (or a .desktop entry) as that user, with the environment its toolkit needs to find
//     the compositor.
//   - ListApps lists the guest's .desktop entries, for host side launchers.
//
// The agent runs as root, the compositor and the applications must not: Wayland clients, the user's D-Bus and
// audio sockets all live in the user's runtime directory.
package seamless

import "fmt"

const (
	// DefaultVsockPort is where the host listens (SeamlessProtocol.defaultVsockPort in Caker).
	DefaultVsockPort = 5100
	// DefaultSocketName is the Wayland socket the compositor creates in the user's runtime directory. It has the
	// standard wayland-N form on purpose: snap's AppArmor profile only lets confined applications (Firefox on
	// Ubuntu) open sockets named like that. 0 is a desktop session's, so a guest that has one is left alone.
	DefaultSocketName = "wayland-1"
	// DefaultBinary is where Install puts the compositor.
	DefaultBinary = "/usr/libexec/caker/caker-seamlessd"
	// BinaryName is the compositor's file name, in assets/<arch>/ and on PATH.
	BinaryName = "caker-seamlessd"
)

// Config says how the compositor is run. The zero value is usable.
type Config struct {
	// User is the account windows and applications belong to. Empty picks the first regular user of the guest.
	User string
	// Port is the host's vsock port.
	Port int
	// SocketName is the Wayland socket name.
	SocketName string
	// Binary is the compositor path.
	Binary string
	// Layout is the XKB layout of the keyboard, which must match the host's (keys are sent by position).
	Layout string
	// ProfilePath is where the shell profile snippet is installed. Empty is DefaultProfilePath.
	ProfilePath string
}

// WithDefaults fills the empty fields.
func (c Config) WithDefaults() Config {
	if c.Port == 0 {
		c.Port = DefaultVsockPort
	}

	if c.SocketName == "" {
		c.SocketName = DefaultSocketName
	}

	if c.Binary == "" {
		c.Binary = DefaultBinary
	}

	if c.ProfilePath == "" {
		c.ProfilePath = DefaultProfilePath
	}

	return c
}

// Connect is the compositor's --connect argument.
func (c Config) Connect() string {
	return fmt.Sprintf("vsock:%d", c.WithDefaults().Port)
}
