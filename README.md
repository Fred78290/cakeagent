# cakeagent

[![Build](https://github.com/Fred78290/cakeagent/actions/workflows/release.yaml/badge.svg)](https://github.com/Fred78290/cakeagent/actions/workflows/release.yaml)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)

Cross-platform gRPC system agent (macOS / Linux) installed by the [Caker](https://github.com/Fred78290/caker) project, exposing guest operations: system information, command execution, shell/TTY, network tunneling, mounting, and service utilities.

## Overview

The repository contains two main implementations:

- **Darwin (Swift)**: `cakeagent` binary in `darwin/Sources/CakeAgent`.
- **Linux (Go)**: `cakeagent` binary in `linux/`.

A Swift test client (`testagent`) is also provided on the Darwin side in `darwin/Sources/TestAgent`.

`CakeAgentLib` is the client library used to communicate with the agent over gRPC.

## Useful layout

- `Package.swift`: SwiftPM configuration (products `cakeagent`, `testagent`, `CakeAgentLib`)
- `darwin/Sources/CakeAgent`: Swift agent server
- `darwin/Sources/TestAgent`: gRPC test CLI client
- `linux/main.go`: Go CLI and agent server
- `proto/agent.proto`: gRPC contract
- `scripts/`: local run scripts
- `_artifacts/`: prebuilt binaries (darwin/linux, amd64/arm64)

## Prerequisites

- macOS 13+ for Darwin builds (Swift 5.10+, via Xcode/Swift toolchain)
- Go (see `linux/go.mod`, modern Go toolchain)
- `protoc` for gRPC stub regeneration

## Build

### Darwin (Swift)

From the repository root:

```bash
swift build
```

Generated binaries:

- `./.build/debug/cakeagent`
- `./.build/debug/testagent`

### Linux (Go)

```bash
cd linux
go build
```

Generated binary:

- `linux/cakeagent`

## Quick start

### Start Swift agent (TLS)

```bash
./scripts/cakeagent-darwin.sh
```

### Start Go agent (TLS)

```bash
./scripts/cakeagent-linux.sh
```

### Start Swift agent (insecure)

```bash
./scripts/runagent-insecure.sh
```

### Run Swift test client

```bash
./scripts/testagent.sh
```

Insecure version:

```bash
./scripts/testagent-insecure.sh
```

## `cakeagent` CLI (Linux / Go)

Main commands:

- `cakeagent serve`: start the server
- `cakeagent version`: display version
- `cakeagent infos [--json]`: system information
- `cakeagent cpu-usage [--json]`: continuous CPU usage
- `cakeagent ping [message] [--json]`: round-trip ping test
- `cakeagent mount <name:target[,opts...]>... [--json]`: VirtioFS mount
- `cakeagent service install|start|stop`: system service management

Global flags:

- `--log-level`
- `--log-format` (`text` or `json`)

`serve` / `service install` flags:

- `--listen` (default `vsock://any:5000`)
- `--ca-cert`
- `--tls-cert`
- `--tls-key`
- `--timeout`
- `--tick`

Example:

```bash
cd linux
./cakeagent serve \
  --listen=tcp://127.0.0.1:5010 \
  --ca-cert="$HOME/.cake/agent/ca.pem" \
  --tls-cert="$HOME/.cake/agent/server.pem" \
  --tls-key="$HOME/.cake/agent/server.key"
```

> Note: on the Go side, flags can be replaced by environment variables prefixed with `CAKEAGENT_`.

## `cakeagent` CLI (Darwin / Swift)

Available commands:

- `cakeagent serve`
- `cakeagent version`
- `cakeagent service install|start|stop`
- `cakeagent resize-disk`
- `cakeagent infos`

Help:

```bash
swift run cakeagent --help
swift run cakeagent serve --help
```

## `testagent` CLI (Darwin / Swift)

gRPC client to test the agent:

```bash
swift run testagent --help
```

Subcommands include: `shell`, `exec`, `run`, `shutdown`, `infos`, `tunnel`, `current`.

## gRPC / Protobuf

The contract is defined in `proto/agent.proto`.

Stub regeneration:

```bash
cd proto
./generate.sh
```

This orchestrates:

- Go generation (`proto/linux.sh`)
- Swift generation (`proto/darwin.sh`)

## Tests

- Swift tests:

```bash
swift test
```

- Additional test utilities in `tests/` (Go/Swift validation clients).

## License

This project is licensed under **GNU AGPL v3**. See `LICENSE`.
