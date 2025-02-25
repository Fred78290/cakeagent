#!/bin/bash
set -e

swift build && exec ./.build/debug/cakeagent run \
	--listen=tcp://127.0.0.1:5000 \
	--ca-cert="${HOME}/.cake/agent/ca.pem" \
	--tls-cert="${HOME}/.cake/agent/server.pem" \
	--tls-key="${HOME}/.cake/agent/server.key"