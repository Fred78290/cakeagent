#!/bin/bash

CURDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PB_RELEASE="29.3"
PB_REL="https://github.com/protocolbuffers/protobuf/releases"

export PROTOC_DIR="/tmp/protoc-${PB_RELEASE}"
export GOPATH=$PROTOC_DIR
export GO111MODULE=on
export PATH=$PROTOC_DIR/bin:$PATH

mkdir -p $PROTOC_DIR

pushd $PROTOC_DIR

go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.3
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

if [ "$(uname)" = "Darwin" ]; then
    PLATFORM=osx-universal_binary
else
    PLATFORM=linux-x86_64
fi

curl -sLO ${PB_REL}/download/v${PB_RELEASE}/protoc-${PB_RELEASE}-${PLATFORM}.zip
unzip -o protoc-${PB_RELEASE}-${PLATFORM}.zip

popd

$PROTOC_DIR/bin/protoc -I . -I vendor --proto_path=proto --go_out=. --go-grpc_out=. agent.proto

sudo rm -rf $PROTOC_DIR

