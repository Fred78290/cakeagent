#!/bin/bash
HERE="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT=${HERE}/..

#	--connect=tcp://127.0.0.1:5000 \

exec ${ROOT}/.build/debug/testagent \
	--connect=unix://${HOME}/.cake/vms/linux.cakedvm/agent.sock \
	--ca-cert="${HOME}/.cake/agent/ca.pem" \
	--tls-cert="${HOME}/.cake/agent/client.pem" \
	--tls-key="${HOME}/.cake/agent/client.key" $@
