#!/bin/bash
VERSION="SNAPSHOT-$(git rev-parse HEAD)"
BUILD_DATE=`date +%Y-%m-%dT%H:%M:%SZ`
LDFLAGS="-s -w -X github.com/Fred78290/cakeagent/version.VERSION=${VERSION} -X github.com/Fred78290/cakeagent/version.BUILD_DATE=${BUILD_DATE}"

mkdir -p _artifacts
pushd linux
GOARCH=amd64 GOOS=linux go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-linux-amd64
GOARCH=arm64 GOOS=linux go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-linux-arm64
GOARCH=amd64 GOOS=darwin go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-darwin-amd64
GOARCH=arm64 GOOS=darwin go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-darwin-arm64

popd

VERSION="SNAPSHOT-$(git rev-parse --short=8 HEAD)"

echo "Creating release ${VERSION}"
exit
gh release create --prerelease  --generate-notes --title "SNAPSHOT ${VERSION}"  ${VERSION} \
	./_artifacts/cakeagent-linux-amd64 \
	./_artifacts/cakeagent-linux-arm64 \
	./_artifacts/cakeagent-darwin-amd64 \
	./_artifacts/cakeagent-darwin-arm64