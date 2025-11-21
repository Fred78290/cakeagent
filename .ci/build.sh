#!/bin/bash
set -ex

CURDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
GITHUB_REF=${GITHUB_REF:=$(git rev-parse --short=8 HEAD)}
VERSION="${GITHUB_REF##*/v}"
BUILD_DATE=`date +%Y-%m-%dT%H:%M:%SZ`

if [ -z "${VERSION}" ]; then
	VERSION="$(git rev-parse --short=8 HEAD)"
fi

export VERSION

TMPFILE=$(mktemp)

envsubst < darwin/Sources/CakeAgent/CI/CI.swift > $TMPFILE
cp $TMPFILE darwin/Sources/CakeAgent/CI/CI.swift

swift build -c release --arch x86_64 --product cakeagent
swift build -c release --arch arm64 --product cakeagent

mkdir -p _artifacts
pushd linux
LDFLAGS="-s -w -X github.com/Fred78290/cakeagent/version.VERSION=${VERSION} -X github.com/Fred78290/cakeagent/version.BUILD_DATE={BUILD_DATE}"

GOARCH=amd64 GOOS=linux go build  -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-linux-amd64
GOARCH=arm64 GOOS=linux go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-linux-arm64
GOARCH=amd64 GOOS=darwin go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-darwin-amd64
GOARCH=arm64 GOOS=darwin go build -ldflags="${LDFLAGS}" -o ../_artifacts/cakeagent-darwin-arm64
popd

if [ ${VERSION} != "$(git rev-parse --short=8 HEAD)" ]; then
	${CURDIR}/setup-keychain.sh
	${CURDIR}/create-pkg.sh
	security delete-keychain build.keychain
	cp -r ${CURDIR}/CakeAgent-${VERSION}.pkg _artifacts
fi
