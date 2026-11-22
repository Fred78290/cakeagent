# caker-seamlessd

Guest half of Caker's seamless windows (the source lives in the cakeagent repository, next to the agent that ships and supervises it; Caker's host side is `SeamlessProtocol.swift` in https://github.com/Fred78290/caker): a headless Wayland compositor (wlroots 0.19, software rendering) that streams
every toplevel and popup to the macOS host over vsock and feeds the host's keyboard and pointer back to the clients.
The host half is `Sources/cakedlib/Seamless/`; the protocol is described in `SEAMLESS_WINDOWS_PLAN.md`.

## Build

```sh
sudo apt install build-essential pkg-config libwlroots-0.19-dev wayland-protocols libwayland-dev \
                 libwayland-bin liblz4-dev libxkbcommon-dev libpixman-1-dev libdrm-dev
make && sudo install caker-seamlessd /usr/local/bin/
```

Needs wlroots 0.19 (Ubuntu 26.04 has it). `make STATIC=1` links everything statically (needs static libraries, see
`scripts/build-seamlessd.sh` in the repository root, which builds them on Alpine); that one binary is what the cakeagent ships. At run time the
guest needs the XKB data files (`xkb-data` on Debian/Ubuntu, `xkeyboard-config` on Fedora/Alpine).

`caker-seamlessd --version` prints the version and the protocol (`caker-seamlessd v1.0.335 (protocol 4)`). CI and `scripts/build-seamlessd.sh`
set it to the release of the agent that embeds the compositor (`make VERSION=...`, `SEAMLESSD_VERSION` for the script and the Dockerfile); a build
made by hand says `dev`. The version is a marker string in the binary, so `cakeagent seamless version` reads it out of the embedded, the
installed and the running compositor (`/proc/<pid>/exe`) without starting any of them.

## Use

The VM needs the vsock bridge to the host socket (`caked launch ... --socket connect://vsock:5100/<host socket path>`),
and a host process listening on that socket (`caked seamless-demo --no-guest --socket <path>` for now).

```sh
caker-seamlessd -- foot            # starts foot with WAYLAND_DISPLAY set, exits with it
caker-seamlessd --socket wayland-1 &   # or run it as a service (caker-seamlessd.service)
WAYLAND_DISPLAY=wayland-1 foot
```

Options: `--connect vsock:PORT|unix:/path|tcp:host:port` (default `vsock:5100`; unix/tcp are for testing), `--socket NAME`,
`-v`. It reconnects every second until the host is there, and re-announces its windows when the host comes back.

Keyboard layout: the host sends physical keys (evdev codes), the guest's layout applies. Set `XKB_DEFAULT_LAYOUT` (and
`_VARIANT`) to the host's.

`SEAMLESS_FORCE_FOCUS=1` gives keyboard focus to every new toplevel, for hosts that can't report window focus.

## Clipboard (protocol 3 text, protocol 4 any MIME type)

Both ways. **Protocol 4** carries a list of MIME typed items: `G_CLIPBOARD_ITEMS` (`0x0A`) and `H_CLIPBOARD_ITEMS` (`0x8D`),
`u16 count` (1 to 16), then per item `u16 mime length` + MIME type + `u32 length` + bytes. A MIME type is `type/subtype` in
printable ASCII without spaces (3 to 127 bytes; the X11 pseudo targets `TARGETS`, `UTF8_STRING`... have no slash and are
not MIME types), the types are distinct (ignoring case), an item is not empty, and the data of all the items together is
at most 32 MiB (`SEAMLESS_MAX_CLIPBOARD_TOTAL`). The parser and the hub refuse anything else, and the connection ends.

When an application sets the selection (copy), the compositor reads what it offers at once, one type after another (a pipe
each, bounded in size and time: 3 s for text, 15 s for the rest, since an X11 application sends a big image in pieces, and
15 s for the whole selection: past that, what was read so far is sent), and sends one `G_CLIPBOARD_ITEMS`: one plain text
first (always named `text/plain;charset=utf-8`, whatever name it was read under: `text/plain`, `UTF8_STRING`...), then every
other type with a valid MIME type, under its own name (a `text/plain` in another charset is sent as that), up to 16 items
and the total above (an item that does not fit, or that nobody answers, is left out). The host's clipboard comes as
`H_CLIPBOARD_ITEMS` and becomes the selection of the seat, offering its types, and the usual names of plain text
(`text/plain;charset=utf-8`, `text/plain`, `UTF8_STRING`, `STRING`, `TEXT`) when it carries UTF-8 text (a text in another
charset is offered under its own type only). Xwayland turns the types into the X11 targets (GIMP reads `image/png`).
The source the host's data came in is recognised when the seat reports the selection back, so it does not go round again.
Primary selection is shared between the guest's applications only. `wlr-data-control` is offered so `wl-copy`/`wl-paste` and
clipboard managers work without a focused surface (which the host decides on). The agent's hub gives the host's clipboard to
the focused user's compositor only, and takes a clipboard from the focused user's compositor only (or from any when no
window has the focus). Xwayland itself only bridges a selection while one of its surfaces has the keyboard focus.

**Protocol 3** (`G_CLIPBOARD` `0x09` / `H_CLIPBOARD` `0x8C`: `u8 format` (0, UTF-8 text) + `u32 length` + bytes, 1 MiB) is
not spoken by this compositor: a link has one version, exactly, so a host older than the agent refuses it, as it did for
each earlier bump (Caker pins the agent it installs). The host still talks protocol 3 to an older agent. What the host does
with the types (which MIME types map to pasteboard types, what is left out) is the host's business: Caker shares text,
images and PDF.

## XWayland (X11 applications)

An application that only speaks X11 (GIMP 2.10, GTK2 and older Qt programs, Wine, many Java and Electron builds) has no
Wayland surface to draw on. The compositor serves an X display through Xwayland: when the guest has the `Xwayland` program
(`/usr/bin/Xwayland`, or `WLR_XWAYLAND`), wlroots listens on a free X display (`:0`, `:1`...) at start and runs Xwayland
only when the first X11 client connects, so a guest that never runs one pays nothing. `SEAMLESS_XWAYLAND=0` turns it off.

The display name is written to `$XDG_RUNTIME_DIR/<wayland socket>.display` (`wayland-1.display` contains `:0`) and set as
`DISPLAY` for the command the compositor runs. The agent reads that file: `cakeagent seamless launch`, the login-shell
profile and the user's D-Bus/systemd activation environment get `DISPLAY`, and with it the toolkit variables become lists
that fall back to X11 (`GDK_BACKEND=wayland,x11`, `QT_QPA_PLATFORM=wayland;xcb`, `SDL_VIDEODRIVER=wayland,x11`). Without
Xwayland they stay Wayland only.

The windows reuse the protocol of the Wayland ones, nothing changed on the wire (protocol 3):

- An X11 window is a toplevel; its title is `WM_NAME`/`_NET_WM_NAME`, its application id `WM_CLASS`, its minimum and maximum
  size come from `WM_NORMAL_HINTS`.
- Every managed window sits at the origin of the X screen and keeps the size it asks for; the host's resize becomes a
  configure of the X window.
- An override-redirect window (menu, tooltip, combo box) is a popup. Its parent is a window of the same process (`_NET_WM_PID`),
  else the focused X11 window, else the newest; its position is the one the client asked for, which is already relative to the
  parent since the parents sit at the origin.
- Focus follows the host: the X window is activated and gets the keyboard; the X11 selection and the Wayland one are bridged
  by wlroots, so the clipboard works between both kinds of applications.

The static build (`scripts/build-seamlessd.sh`) links the xcb libraries wlroots needs for it and builds libXau, which Alpine
only has as a shared library. The `Xwayland` program itself is not embedded: it comes from the guest (the package is
`xwayland` on Debian, Ubuntu, openSUSE and Alpine, `xorg-x11-server-Xwayland` on Fedora and Red Hat; Caker's install
scripts install it when seamless windows are on).

Not done for X11 windows: the cursor an X11 client draws is not forwarded (the standard shapes are), a window is always at
the origin so two windows that position themselves relative to each other (rare) lose that, the host's scale is applied by
stretching (X11 clients are not scale-aware, so Retina displays show them softer), maximize/fullscreen/minimize requests are
ignored like the xdg ones.

## Verified

Ubuntu 26.04 arm64 guest in Caker, host `caked seamless-demo --no-guest` over the real vsock bridge:
`weston-simple-shm` (animated) and `foot` (a shell: typed keys reach the guest, output is drawn back). Clipboard: a test host
speaking the protocol, `wl-copy`/`wl-paste`, both directions, 900 KB from the host, a 1.1 MB copy dropped; and from the
guest to a real host through the hub. Any MIME type: the hub and the codec by unit tests, and a test host speaking protocol 4 with GTK X11 clients through Xwayland (text, `text/html` and `image/png` read by the client; the same three set by the client and sent to the host). XWayland: `xclock` and GIMP 2.10.36 (GTK2, X11 only) started through the agent show as
windows on the host (`[linux] xclock`, `[linux] GNU Image Manipulation Program`, 1024x800).

## Not done

Clipboard files and lazy fetching of big types (everything offered is read at once), client-drawn cursor images, IME (`text-input`), popup grabs against real toolkits
(code exists, untested), `wl_output` changes when the host moves a window between displays, GPU acceleration.
