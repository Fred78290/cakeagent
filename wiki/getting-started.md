<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Démarrage

Dans l'usage normal, **Caker installe l'agent pour vous** (cloud-init ou SSH selon l'image) et le met à jour quand sa version change : il n'y a rien à faire à la main. Cette page sert à compiler et lancer l'agent soi-même, pour le développement ou pour un autre client que Caker.

## Prérequis

- macOS 13+ pour les builds Darwin (Swift 5.10+, via Xcode ou la chaîne d'outils Swift)
- Go 1.26+ pour les builds Linux et Windows (voir `linux/go.mod`)
- `protoc` pour régénérer les stubs gRPC

## Compiler

Agent Swift (macOS), depuis la racine du dépôt :

```bash
swift build
# ./.build/debug/cakeagent   ./.build/debug/testagent
```

Agent Go (Linux, Windows, et Darwin en croisé) :

```bash
cd linux
go build                      # linux/cakeagent
GOARCH=arm64 GOOS=linux go build -o ../_artifacts/cakeagent-linux-arm64
```

Le module Go se compile pour `linux`, `darwin` et `windows` en `amd64` et `arm64`. Le binaire macOS signé et notarisé des versions est le build Swift.

Pour qu'un binaire Go annonce une version, passez-la à l'édition de liens :

```bash
go build -ldflags="-X github.com/Fred78290/cakeagent/version.VERSION=v1.0.310 -X github.com/Fred78290/cakeagent/version.BUILD_DATE=$(date +%Y-%m-%dT%H:%M:%SZ)"
```

Sans cela, `cakeagent version` affiche `v0.0.0-unset`.

## Certificats

Avec TLS mutuel, l'agent exige trois fichiers : `--ca-cert`, `--tls-cert`, `--tls-key`. Les scripts de `scripts/` les cherchent dans `~/.cake/agent/` (`ca.pem`, `server.pem`, `server.key`, `client.pem`, `client.key`). Sans ces options, l'agent tourne sans sécurité de transport : à réserver au développement local. Caker génère ses propres certificats et les installe avec l'agent.

## Lancer l'agent

```bash
./scripts/cakeagent-darwin.sh      # agent Swift (TLS)
./scripts/cakeagent-linux.sh       # agent Go (TLS)
./scripts/runagent-insecure.sh     # agent Swift (sans TLS)
```

Ou directement :

```bash
cd linux
./cakeagent serve \
  --listen=tcp://127.0.0.1:5010 \
  --ca-cert="$HOME/.cake/agent/ca.pem" \
  --tls-cert="$HOME/.cake/agent/server.pem" \
  --tls-key="$HOME/.cake/agent/server.key"
```

Dans un invité, l'adresse par défaut est `vsock://any:5000` : l'hôte joint l'agent par le socket vsock de la VM (voir [Architecture](architecture)).

## Tester avec le client de référence

```bash
./scripts/testagent.sh             # client Swift contre un agent TLS
./scripts/testagent-insecure.sh    # contre un agent sans TLS
./scripts/testagent-linux.sh       # par socket unix, contre une VM qui tourne
swift run testagent --help
```

## Installer comme service

```bash
cakeagent service install --listen=vsock://any:5000 \
  --ca-cert /etc/cakeagent/ssl/ca.pem \
  --tls-cert /etc/cakeagent/ssl/server.pem \
  --tls-key  /etc/cakeagent/ssl/server.key
cakeagent service start
cakeagent service stop
cakeagent service remove
```

`install` enregistre un service systemd ou OpenRC (Linux), launchd (macOS) ou un service Windows, puis le démarre. Toute option peut aussi venir d'une variable d'environnement préfixée `CAKEAGENT_` (`CAKEAGENT_LISTEN`, `CAKEAGENT_LOG_LEVEL`…). La liste complète est dans la [référence CLI](cli-reference).

</div>

<div class="lang-en" style="display:block" markdown="1">

# Getting started

In normal use **Caker installs the agent for you** (through cloud-init or SSH depending on the image) and updates it when its version changes: there is nothing to do by hand. This page is for building and running the agent yourself, for development or for a client other than Caker.

## Prerequisites

- macOS 13+ for Darwin builds (Swift 5.10+, through Xcode or the Swift toolchain)
- Go 1.26+ for Linux and Windows builds (see `linux/go.mod`)
- `protoc` to regenerate the gRPC stubs

## Build

Swift agent (macOS), from the repository root:

```bash
swift build
# ./.build/debug/cakeagent   ./.build/debug/testagent
```

Go agent (Linux, Windows, and Darwin cross-built):

```bash
cd linux
go build                      # linux/cakeagent
GOARCH=arm64 GOOS=linux go build -o ../_artifacts/cakeagent-linux-arm64
```

The Go module builds for `linux`, `darwin` and `windows` on `amd64` and `arm64`. The signed, notarized macOS binary shipped in releases is the Swift build.

For a Go binary to report a version, pass it at link time:

```bash
go build -ldflags="-X github.com/Fred78290/cakeagent/version.VERSION=v1.0.310 -X github.com/Fred78290/cakeagent/version.BUILD_DATE=$(date +%Y-%m-%dT%H:%M:%SZ)"
```

Without it `cakeagent version` prints `v0.0.0-unset`.

## Certificates

With mutual TLS the agent needs three files: `--ca-cert`, `--tls-cert`, `--tls-key`. The scripts in `scripts/` look for them in `~/.cake/agent/` (`ca.pem`, `server.pem`, `server.key`, `client.pem`, `client.key`). Without these options the agent runs with no transport security: for local development only. Caker generates its own certificates and installs them with the agent.

## Run the agent

```bash
./scripts/cakeagent-darwin.sh      # Swift agent (TLS)
./scripts/cakeagent-linux.sh       # Go agent (TLS)
./scripts/runagent-insecure.sh     # Swift agent (no TLS)
```

Or directly:

```bash
cd linux
./cakeagent serve \
  --listen=tcp://127.0.0.1:5010 \
  --ca-cert="$HOME/.cake/agent/ca.pem" \
  --tls-cert="$HOME/.cake/agent/server.pem" \
  --tls-key="$HOME/.cake/agent/server.key"
```

In a guest the default address is `vsock://any:5000`: the host reaches the agent through the VM's vsock socket (see [Architecture](architecture)).

## Try it with the reference client

```bash
./scripts/testagent.sh             # Swift client against a TLS agent
./scripts/testagent-insecure.sh    # against an agent without TLS
./scripts/testagent-linux.sh       # over a unix socket, against a running VM
swift run testagent --help
```

## Install as a service

```bash
cakeagent service install --listen=vsock://any:5000 \
  --ca-cert /etc/cakeagent/ssl/ca.pem \
  --tls-cert /etc/cakeagent/ssl/server.pem \
  --tls-key  /etc/cakeagent/ssl/server.key
cakeagent service start
cakeagent service stop
cakeagent service remove
```

`install` registers a systemd or OpenRC service (Linux), a launchd service (macOS) or a Windows service, then starts it. Any option can also come from an environment variable prefixed with `CAKEAGENT_` (`CAKEAGENT_LISTEN`, `CAKEAGENT_LOG_LEVEL`…). The full list is in the [CLI reference](cli-reference).

</div>
