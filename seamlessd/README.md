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

## Clipboard (protocol 3)

Text only, both ways, up to 1 MiB (`SEAMLESS_MAX_CLIPBOARD`). When an application sets the selection (copy), the compositor
reads its `text/plain` at once (a pipe, bounded in size and time, 3 s) and sends it as `G_CLIPBOARD` (`0x09`: `u8 format`
(0, UTF-8 text) + `u32 length` + bytes). The host's clipboard comes as `H_CLIPBOARD` (`0x8C`, same payload) and becomes the
selection of the seat, offered as `text/plain;charset=utf-8`, `text/plain`, `UTF8_STRING`, `STRING` and `TEXT`; the source it
came in is recognised when the seat reports the selection back, so it does not go round again. Primary selection is shared
between the guest's applications only. `wlr-data-control` is offered so `wl-copy`/`wl-paste` and clipboard managers work
without a focused surface (which the host decides on). The agent's hub gives the host's clipboard to the focused user's
compositor only, and takes a clipboard from the focused user's compositor only (or from any when no window has the focus).
A host on protocol 2 (or a compositor on protocol 2) has none of this: the host simply doesn't send the message.

## Verified

Ubuntu 26.04 arm64 guest in Caker, host `caked seamless-demo --no-guest` over the real vsock bridge:
`weston-simple-shm` (animated) and `foot` (a shell: typed keys reach the guest, output is drawn back). Clipboard: a test host
speaking the protocol, `wl-copy`/`wl-paste`, both directions, 900 KB from the host, a 1.1 MB copy dropped; and from the
guest to a real host through the hub.

## Not done

XWayland (X11 apps), images and other formats on the clipboard, client-drawn cursor images, IME (`text-input`), popup grabs against real toolkits
(code exists, untested), `wl_output` changes when the host moves a window between displays, GPU acceleration.
