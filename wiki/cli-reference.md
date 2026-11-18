<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Référence CLI

Toute option peut être donnée par une variable d'environnement préfixée `CAKEAGENT_` (`CAKEAGENT_LISTEN`, `CAKEAGENT_LOG_LEVEL`…).

## `cakeagent` (Go : Linux, Windows)

```
cakeagent
├── serve                                     démarre le serveur gRPC
├── version                                   affiche la version
├── infos [--json]                            informations système
├── cpu-usage [--json]                        usage CPU en continu
├── ping [message] [--json]                   test aller-retour
├── mount <nom:cible[,opts...]>... [--json]   monte des partages VirtioFS
├── seamless …                                compositeur de fenêtres (Linux, voir Fenêtres seamless)
├── display-watch                             caché, Windows : lancé par le service dans la session de l'utilisateur
└── service
    ├── install                               installe le service et le démarre
    ├── remove                                supprime le service
    ├── start                                 démarre le service installé
    └── stop                                  arrête le service installé
```

Options globales : `--log-level` (`panic|debug|info|warning|error|fatal`) et `--log-format` (`text|json`).

`cakeagent version` affiche `The current version is: <version>, build at: <date>`, et c'est aussi la valeur de `agentVersion` dans la réponse `Info` : Caker s'en sert pour savoir si l'agent d'un invité doit être mis à jour (voir [Versions et publication](releases)).

### Options de `serve` et `service install`

| Option | Défaut | Description |
|---|---|---|
| `--listen` | Linux et macOS : `vsock://any:5000`<br>Windows : `tcp://127.0.0.1:5000` (utiliser `vsock://any:5000` pour être joignable depuis l'hôte) | Adresse d'écoute (voir [Architecture](architecture)) |
| `--ca-cert` | — | Certificat de la CA pour le TLS mutuel |
| `--tls-cert` | — | Certificat TLS du serveur |
| `--tls-key` | — | Clé privée du serveur |
| `--timeout` | `120s` | Délai d'une requête (`0s` : pas de délai) |
| `--tick` | `1s` | Intervalle de l'événement de tick |
| `--mount` (`service install` seulement) | — | Montage `nom:cible[,opts...]`, répétable, configuré à l'installation |
| `--seamless` (Linux) | `false` | Lance le compositeur de fenêtres à côté de l'agent |
| `--seamless-user`, `--seamless-port`, `--seamless-layout`, `--seamless-binary` (Linux) | premier utilisateur normal, `5100`, —, `/usr/libexec/caker/caker-seamlessd` | Compte du compositeur, port vsock de l'hôte, disposition XKB et chemin du binaire |

### Format de `mount`

`nom:cible[,uid=X,gid=Y,ro,early]` :

```bash
cakeagent mount share:/mnt/share
cakeagent mount data:/data,uid=1000,gid=1000
cakeagent mount config:/etc/config,ro,early share:/mnt/share,uid=1000
```

Les montages ne sont pas pris en charge sous Windows.

## `cakeagent` (Swift : macOS)

```
cakeagent
├── serve            démarre le serveur gRPC (géré par launchd une fois installé)
├── version          affiche la version
├── infos            informations système
├── resize-disk      agrandit le disque macOS à tout l'espace disponible
└── service
    ├── install       installe un service launchd
    ├── start         démarre le service installé
    └── stop          arrête le service installé
```

`serve` et `service install` ont les mêmes options que l'agent Go (`--listen`, `--ca-cert`, `--tls-cert`, `--tls-key`, `--log-level`), et `service install` ajoute `--insecure` pour se passer de TLS.

## `testagent` (Swift)

Client gRPC de référence : `shell` (TTY interactif par `Execute`), `exec`, `run`, `shutdown`, `infos`, `current` (usage CPU en direct), `tunnel local` et `tunnel remote` (redirection de ports). `swift run testagent --help` détaille chaque sous-commande.

</div>

<div class="lang-en" style="display:block" markdown="1">

# CLI reference

Any flag can be given as an environment variable prefixed with `CAKEAGENT_` (`CAKEAGENT_LISTEN`, `CAKEAGENT_LOG_LEVEL`…).

## `cakeagent` (Go: Linux, Windows)

```
cakeagent
├── serve                                     start the gRPC server
├── version                                   display version information
├── infos [--json]                            display system information
├── cpu-usage [--json]                        display continuous CPU usage
├── ping [message] [--json]                   round-trip ping test
├── mount <name:target[,opts...]>... [--json] mount VirtioFS endpoints
├── seamless …                                windows compositor (Linux, see Seamless windows)
├── display-watch                             hidden, Windows: run by the service in the user's session
└── service
    ├── install                               install the service and start it
    ├── remove                                remove the service
    ├── start                                 start the installed service
    └── stop                                  stop the installed service
```

Global flags: `--log-level` (`panic|debug|info|warning|error|fatal`) and `--log-format` (`text|json`).

`cakeagent version` prints `The current version is: <version>, build at: <date>`, and that version is also the `agentVersion` of the `Info` reply: Caker uses it to tell whether a guest's agent must be updated (see [Versions and releases](releases)).

### `serve` and `service install` flags

| Flag | Default | Description |
|---|---|---|
| `--listen` | Linux and macOS: `vsock://any:5000`<br>Windows: `tcp://127.0.0.1:5000` (use `vsock://any:5000` to be reachable from the host) | Listen address (see [Architecture](architecture)) |
| `--ca-cert` | — | CA certificate for mutual TLS |
| `--tls-cert` | — | Server TLS certificate |
| `--tls-key` | — | Server private key |
| `--timeout` | `120s` | Request timeout (`0s`: no timeout) |
| `--tick` | `1s` | Tick event interval |
| `--mount` (`service install` only) | — | Repeatable `name:target[,opts...]` mount, configured at install time |
| `--seamless` (Linux) | `false` | Run the windows compositor next to the agent |
| `--seamless-user`, `--seamless-port`, `--seamless-layout`, `--seamless-binary` (Linux) | first regular user, `5100`, —, `/usr/libexec/caker/caker-seamlessd` | Compositor account, host vsock port, XKB layout and binary path |

### `mount` format

`name:target[,uid=X,gid=Y,ro,early]`:

```bash
cakeagent mount share:/mnt/share
cakeagent mount data:/data,uid=1000,gid=1000
cakeagent mount config:/etc/config,ro,early share:/mnt/share,uid=1000
```

Mounts are not supported on Windows.

## `cakeagent` (Swift: macOS)

```
cakeagent
├── serve            start the gRPC server (launchd-managed once installed)
├── version          display version information
├── infos            display system information
├── resize-disk      resize the macOS disk to use all available space
└── service
    ├── install       install as a launchd service
    ├── start         start the installed service
    └── stop          stop the installed service
```

`serve` and `service install` take the same flags as the Go agent (`--listen`, `--ca-cert`, `--tls-cert`, `--tls-key`, `--log-level`), and `service install` adds `--insecure` to skip TLS.

## `testagent` (Swift)

The reference gRPC client: `shell` (interactive TTY through `Execute`), `exec`, `run`, `shutdown`, `infos`, `current` (live CPU usage), `tunnel local` and `tunnel remote` (port forwarding). `swift run testagent --help` details each subcommand.

</div>
