# Static caker-seamlessd for the architecture this image is built on, with the XKB data it compiles its keymap from:
#   docker build -f scripts/seamlessd.Dockerfile --output type=local,dest=out .
# leaves out/caker-seamlessd and out/xkb/. The compositor needs the data (libxkbcommon reads rules, keycodes, types,
# compat and symbols files at run time), the agent installs it beside the binary for guests without xkb-data.
FROM alpine:3.24 AS build
RUN apk add --no-cache file xkeyboard-config
COPY scripts/build-seamlessd.sh /build-seamlessd.sh
COPY seamlessd /src
RUN /build-seamlessd.sh /src /caker-seamlessd

# Only what libxkbcommon reads: no geometry, no rules registry (xml, lst). Dereferenced, the distribution links some.
RUN set -eu; \
	src=$(ls -d /usr/share/xkeyboard-config-2 /usr/share/X11/xkb 2>/dev/null | head -n1); \
	mkdir /xkb; \
	for d in rules keycodes types compat symbols; do cp -RL "$src/$d" /xkb/; done; \
	find /xkb -name '*.xml' -o -name '*.lst' | xargs -r rm -f; \
	test -s /xkb/rules/evdev

FROM scratch
COPY --from=build /caker-seamlessd /caker-seamlessd
COPY --from=build /xkb /xkb
