#!/bin/bash
set -e

swift build && exec ./.build/debug/cakeagent run --listen=tcp://127.0.0.1:5000
