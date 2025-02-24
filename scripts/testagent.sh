#!/bin/bash

exec ./.build/debug/testagent \
	--ca-cert="${HOME}/.cake/agent/ca.pem" \
	--tls-cert="${HOME}/.cake/agent/client.pem" \
	--tls-key="${HOME}/.cake/agent/client.key" $@
