#!/bin/bash
set -e

cd linux 2>&1
go build && exec ./cakeagent --listen=tcp://127.0.0.1:5000
