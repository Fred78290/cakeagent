<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Développement

## Compiler et tester

```bash
# Agent Swift, bibliothèque et client de référence (macOS)
swift build
swift test

# Agent Go
cd linux
go build
go vet ./...
go test ./...
```

Les tests Go qui changent d'utilisateur (`shell_user_*`, `pkg/seamless`) doivent tourner **sans être root** : une exécution en root échoue par conception (un compte root est refusé pour le compositeur et le hub). Pour tester sous Linux depuis un Mac, compilez le binaire de test en croisé puis copiez-le dans une VM :

```bash
GOOS=linux GOARCH=arm64 go test -c ./pkg/seamless -o seamless.test
```

Le module n'est pas compilable pour FreeBSD (`resize` et vsock n'y existent pas) ; seuls `linux`, `darwin` et `windows` sont pris en charge.

Les clients autonomes de `tests/` (`test_client.go`, `test_client_swift.swift`, `test_swift_cpu.go`, `test_parser.go`) exercent un agent en marche par gRPC.

## Modifier le contrat gRPC

Éditez `proto/agent.proto`, puis régénérez les stubs Go et Swift :

```bash
cd proto
./generate.sh        # appelle linux.sh (Go) et darwin.sh (Swift)
```

Mettez ensuite à jour **les deux** serveurs (Go dans `linux/pkg`, Swift dans `darwin/Sources/CakeAgent`) et la bibliothèque cliente `CakeAgentLib`. Un champ ajouté n'est lu que par un agent qui le connaît : un agent plus ancien l'ignore, ce qui est voulu pour les invités non mis à jour.

## Le compositeur seamless

Les binaires Linux embarquent `caker-seamlessd`, construit à part :

```bash
scripts/build-seamlessd.sh      # voir aussi scripts/seamlessd.Dockerfile
```

Le workflow `seamlessd.yaml` le construit pour `amd64` et `arm64` à partir de ses sources (répertoire `seamlessd/` de ce dépôt) et les workflows Linux téléchargent les artefacts dans `linux/pkg/seamless/assets/<arch>/` avant de compiler. Un build local sans ces fichiers n'embarque pas de compositeur (voir [Fenêtres seamless](seamless-windows)).

## CI

- `linux.yaml` : binaires Go (Linux, Darwin, Windows) ; étiquettes `ci-linux` et `ci-release`.
- `release.yaml` : agent Swift, signature, notarisation et `.pkg` macOS ; étiquettes `ci-build` et `v*`.
- `seamlessd.yaml` : compositeur, appelé par `linux.yaml` ou lancé à la main.
- `publish-wiki.yaml` : publie le dossier `wiki/` dans le wiki GitHub.

Le détail des versions est dans [Versions et publication](releases).

## Wiki

Les pages sont dans `wiki/` (français et anglais dans un même fichier, comme pour Caker). À chaque poussée sur `main` qui touche `wiki/`, `publish-wiki.yaml` les publie dans le wiki GitHub ; à la main : `./scripts/publish-wiki.sh Fred78290/cakeagent`. La publication utilise le secret `WIKI_TOKEN` (un jeton qui peut pousser dans le wiki), et le wiki doit avoir été initialisé une fois dans l'interface GitHub (une première page créée).

</div>

<div class="lang-en" style="display:block" markdown="1">

# Development

## Build and test

```bash
# Swift agent, library and reference client (macOS)
swift build
swift test

# Go agent
cd linux
go build
go vet ./...
go test ./...
```

The Go tests that switch user (`shell_user_*`, `pkg/seamless`) must run **as a non-root user**: as root they fail by design (a root account is refused for the compositor and the hub). To test on Linux from a Mac, cross-compile the test binary and copy it into a VM:

```bash
GOOS=linux GOARCH=arm64 go test -c ./pkg/seamless -o seamless.test
```

The module does not build for FreeBSD (`resize` and vsock don't exist there); only `linux`, `darwin` and `windows` are supported.

The standalone clients in `tests/` (`test_client.go`, `test_client_swift.swift`, `test_swift_cpu.go`, `test_parser.go`) exercise a running agent over gRPC.

## Changing the gRPC contract

Edit `proto/agent.proto`, then regenerate the Go and Swift stubs:

```bash
cd proto
./generate.sh        # calls linux.sh (Go) and darwin.sh (Swift)
```

Then update **both** servers (Go in `linux/pkg`, Swift in `darwin/Sources/CakeAgent`) and the `CakeAgentLib` client library. A field you add is only read by an agent that knows it: an older agent ignores it, which is intended for guests that haven't been updated.

## The seamless compositor

The Linux binaries embed `caker-seamlessd`, built separately:

```bash
scripts/build-seamlessd.sh      # see also scripts/seamlessd.Dockerfile
```

The `seamlessd.yaml` workflow builds it for `amd64` and `arm64` from its sources (the `seamlessd/` directory of this repository) and the Linux workflows download the artifacts into `linux/pkg/seamless/assets/<arch>/` before compiling. A local build without those files embeds no compositor (see [Seamless windows](seamless-windows)).

## CI

- `linux.yaml`: Go binaries (Linux, Darwin, Windows); tags `ci-linux` and `ci-release`.
- `release.yaml`: Swift agent, signing, notarization and the macOS `.pkg`; tags `ci-build` and `v*`.
- `seamlessd.yaml`: the compositor, called by `linux.yaml` or run by hand.
- `publish-wiki.yaml`: publishes the `wiki/` folder to the GitHub wiki.

Versioning details are in [Versions and releases](releases).

## Wiki

The pages live in `wiki/` (French and English in one file, like Caker). On every push to `main` that touches `wiki/`, `publish-wiki.yaml` publishes them to the GitHub wiki; by hand: `./scripts/publish-wiki.sh Fred78290/cakeagent`. Publishing uses the `WIKI_TOKEN` secret (a token that can push to the wiki), and the wiki must have been initialized once in the GitHub UI (a first page created).

</div>
