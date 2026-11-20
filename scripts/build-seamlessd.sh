#!/bin/sh
# Builds a fully static caker-seamlessd (Caker's seamless windows compositor) on Alpine Linux (musl), so one
# binary runs in any Linux guest of the same architecture whatever libraries it has.
#
#   scripts/build-seamlessd.sh <compositor source dir> <output file>
#
# The source is the seamlessd/ directory of this repository. Run it on Alpine (a container or a
# VM) of the architecture to build for; scripts/seamlessd.Dockerfile does that. wlroots is built from source with
# only the headless backend and the pixman renderer, the distribution's wlroots drags in DRM, libinput, X11,
# Vulkan... which have no static libraries. XWayland support is kept (the xcb libraries are linked in); the Xwayland
# program itself comes from the guest (its xwayland package), the compositor starts it when an X11 application connects.
set -eu

SRC=${1:?usage: build-seamlessd.sh <source dir> <output file>}
OUT=${2:?usage: build-seamlessd.sh <source dir> <output file>}

WLROOTS_VERSION=0.19.3
WLROOTS_SHA256=5d02693175e5afd9af5f10e3e4976d6e9249dc39a90eb17d23fa5f54b125ccc5
LIBDRM_VERSION=2.4.125
LIBDRM_SHA256=d4bae92797a50f81a93524762e0410a49cd84cfa0f997795bc0172ac8fb1d96a
LIBXAU_VERSION=1.0.12
LIBXAU_SHA256=74d0e4dfa3d39ad8939e99bda37f5967aba528211076828464d2777d477fc0fb

PREFIX=/opt/seamlessd-deps
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

apk add --no-cache build-base pkgconf meson ninja curl file \
	wayland-dev wayland-static wayland-protocols \
	libxkbcommon-dev libxkbcommon-static pixman-dev pixman-static \
	lz4-dev lz4-static expat-static libffi-dev \
	libxcb-dev libxcb-static xcb-util-wm-dev libxdmcp-dev xorgproto xwayland-dev

fetch() { # url sha256 file
	curl -fsSL -o "$WORK/$3" "$1"
	echo "$2  $WORK/$3" | sha256sum -c -
}

fetch "https://dri.freedesktop.org/libdrm/libdrm-$LIBDRM_VERSION.tar.xz" "$LIBDRM_SHA256" libdrm.tar.xz
fetch "https://www.x.org/releases/individual/lib/libXau-$LIBXAU_VERSION.tar.xz" "$LIBXAU_SHA256" libXau.tar.xz
fetch "https://gitlab.freedesktop.org/wlroots/wlroots/-/releases/$WLROOTS_VERSION/downloads/wlroots-$WLROOTS_VERSION.tar.gz" "$WLROOTS_SHA256" wlroots.tar.gz

(
	cd "$WORK" && tar xJf libdrm.tar.xz && cd "libdrm-$LIBDRM_VERSION"
	meson setup build --prefix="$PREFIX" --buildtype=release -Ddefault_library=static \
		-Dintel=disabled -Dradeon=disabled -Damdgpu=disabled -Dnouveau=disabled -Dvmwgfx=disabled \
		-Dfreedreno=disabled -Dvc4=disabled -Detnaviv=disabled -Dexynos=disabled -Domap=disabled \
		-Dtegra=disabled -Dcairo-tests=disabled -Dtests=false -Dman-pages=disabled -Dvalgrind=disabled
	meson install -C build
)

# libxcb's static library needs libXau, which Alpine only ships as a shared one (libxdmcp-dev has the static libXdmcp).
(
	cd "$WORK" && tar xJf libXau.tar.xz && cd "libXau-$LIBXAU_VERSION"
	./configure --prefix="$PREFIX" --enable-static --disable-shared
	make -j"$(nproc)"
	make install
)

# wlroots (and later the compositor) find the libdrm built above, and wlroots itself, through pkg-config.
export PKG_CONFIG_PATH="$PREFIX/lib/pkgconfig"

(
	cd "$WORK" && tar xzf wlroots.tar.gz && cd "wlroots-$WLROOTS_VERSION"
	meson setup build --prefix="$PREFIX" --buildtype=release -Ddefault_library=static \
		-Dxwayland=enabled -Dxcb-errors=disabled -Dbackends=[] -Drenderers=[] -Dallocators=[] -Dsession=disabled \
		-Dcolor-management=disabled -Dlibliftoff=disabled -Dexamples=false
	meson install -C build
)

cp -R "$SRC" "$WORK/compositor"
make -C "$WORK/compositor" STATIC=1
# SEAMLESSD_KEEP_SYMBOLS=1 keeps the symbols (a binary to debug with gdb); the one the agent embeds is stripped.
if [ "${SEAMLESSD_KEEP_SYMBOLS:-0}" = 1 ]; then
	cp "$WORK/compositor/caker-seamlessd" "$OUT"
else
	strip -o "$OUT" "$WORK/compositor/caker-seamlessd"
fi

# It must not need a dynamic loader (static-pie reports one in `ldd` on musl, `file` is authoritative).
file "$OUT" 2>/dev/null | grep -q "statically linked\|static-pie" || { echo "$OUT is not static" >&2; exit 1; }
ls -l "$OUT"
