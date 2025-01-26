#!/bin/bash
set -ex

CURDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
GITHUB_REF=${GITHUB_REF:=$(git rev-parse HEAD)}
VERSION="${GITHUB_REF##*/}"

if [ -z "${VERSION}" ]; then
	VERSION="SNAPSHOT"
else
	VERSION="${VERSION##*/v}"
fi

export VERSION

TMPFILE=$(mktemp)

envsubst < darwin/Sources/CakeAgent/CI/CI.swift > $TMPFILE
cp $TMPFILE darwin/Sources/CakeAgent/CI/CI.swift
cp $TMPFILE darwin/Sources/CakeAgent/CI/CI.swift

swift build -c release --arch x86_64 --product cakeagent
swift build -c release --arch arm64 --product cakeagent

mkdir -p _artifacts
pushd linux
GOARCH=amd64 GOOS=linux go build -o ../_artifacts/cakeagent-linux-amd64
GOARCH=arm64 GOOS=linux go build -o ../_artifacts/cakeagent-linux-arm64
GOARCH=amd64 GOOS=darwin go build -o ../_artifacts/cakeagent-darwin-amd64
GOARCH=arm64 GOOS=darwin go build -o ../_artifacts/cakeagent-darwin-arm64
popd

if [ ${VERSION} != "SNAPSHOT" ]; then
	${CURDIR}/setup-keychain.sh
	${CURDIR}/create-pkg.sh
	security delete-keychain build.keychain
	cp -r ${CURDIR}/CakeAgent-${VERSION}.pkg _artifacts
fi
