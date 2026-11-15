# Static caker-seamlessd for the architecture this image is built on:
#   docker build -f scripts/seamlessd.Dockerfile --build-context compositor=<caker>/Tools/SeamlessCompositor \
#          --output type=local,dest=out .
# leaves out/caker-seamlessd.
FROM alpine:3.24 AS build
RUN apk add --no-cache file
COPY scripts/build-seamlessd.sh /build-seamlessd.sh
COPY --from=compositor . /src
RUN /build-seamlessd.sh /src /caker-seamlessd

FROM scratch
COPY --from=build /caker-seamlessd /caker-seamlessd
