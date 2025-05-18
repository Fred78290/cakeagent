#!/bin/bash
HERE="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT=${HERE}/..

exec ${ROOT}/.build/debug/cakeagent run --listen=tcp://127.0.0.1:5010 \
	--ca-cert="${HOME}/.cake/agent/ca.pem" \
	--tls-cert="${HOME}/.cake/agent/server.pem" \
	--tls-key="${HOME}/.cake/agent/server.key"