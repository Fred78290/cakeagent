#!/bin/bash
set -e

cd linux 2>&1
go build && exec ./cakeagent \
	--listen=tcp://127.0.0.1:5010 --log-level=trace \
	--ca-cert="${HOME}/.cake/agent/ca.pem" \
	--tls-cert="${HOME}/.cake/agent/server.pem" \
	--tls-key="${HOME}/.cake/agent/server.key"
