<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Versions et publication

## Schéma de version

Une version de cakeagent s'écrit `v<BASE_VERSION>.<nombre de commits>` : `BASE_VERSION` est la variable de dépôt du même nom (`1.0` si elle n'est pas définie) et le nombre est `git rev-list --count HEAD`, comme pour Caker. Exemple : `v1.0.310`. C'est à la fois le nom de la version GitHub, l'étiquette Git et la valeur que l'agent annonce (`cakeagent version`, champ `agentVersion` de `Info`).

Le compte exige l'historique complet : les workflows clonent avec `fetch-depth: 0`. Les anciennes versions s'appellent `SNAPSHOT-<sha sur 8 caractères>`.

## Déclencheurs CI

| Étiquette poussée | Workflow | Résultat |
|---|---|---|
| `ci-linux` | `linux.yaml` | Construit les six binaires (Linux, Darwin, Windows × `amd64`, `arm64`) et publie une **préversion** `v<BASE>.<n>` |
| `ci-release` | `linux.yaml` | Même construction ; **promeut** la préversion du même commit si elle existe (binaires rafraîchis, marquée « latest »), sinon crée la version |
| `ci-build` | `release.yaml` | Construit l'agent Swift et le paquet macOS, publie une préversion |
| `v*` | `release.yaml` | Version avec le `.pkg` macOS signé et notarisé ; la version embarquée est le nom de l'étiquette |

`ci-linux` puis `ci-release` sur le même commit partagent une seule version : la seconde réutilise la version existante au lieu d'échouer. Si l'étiquette `v…` existe déjà sur un autre commit, la publication est refusée.

Les binaires publiés sont nommés `cakeagent-<os>-<arch>` (avec `.exe` pour Windows) : `cakeagent-linux-amd64`, `cakeagent-linux-arm64`, `cakeagent-darwin-amd64`, `cakeagent-darwin-arm64`, `cakeagent-windows-amd64.exe`, `cakeagent-windows-arm64.exe`. Les binaires Linux embarquent le compositeur seamless, construit d'abord par `seamlessd.yaml`.

## Comment Caker choisit sa version

Caker fixe la version de l'agent qu'il installe dans la constante `CAKEAGENT_VERSION` (`Sources/cakedlib/CloudImage/CloudInit.swift`), qui est **le nom complet de la version** :

```
https://github.com/Fred78290/cakeagent/releases/download/<CAKEAGENT_VERSION>/cakeagent-<os>-<arch>
```

Pour qu'une nouvelle version de l'agent arrive dans les invités :

1. fusionner le changement dans cakeagent, pousser `ci-linux` (ou `ci-release`) et attendre la fin du workflow ;
2. dans Caker, mettre `CAKEAGENT_VERSION` à la nouvelle version (`v1.0.310`, par exemple).

Caker compare `CAKEAGENT_VERSION` à l'`agentVersion` annoncée par chaque invité, **à l'identique** : un invité dont l'agent est différent affiche « Update agent », et l'installation par SSH ou cloud-init remplace l'agent. La comparaison exacte évite de prendre `v1.0.12` pour installé sur un invité en `v1.0.123`.

</div>

<div class="lang-en" style="display:block" markdown="1">

# Versions and releases

## Version scheme

A cakeagent version is `v<BASE_VERSION>.<commit count>`: `BASE_VERSION` is the repository variable of the same name (`1.0` when unset) and the count is `git rev-list --count HEAD`, like Caker. For example `v1.0.310`. It is the name of the GitHub release, the Git tag and the version the agent reports (`cakeagent version`, the `agentVersion` field of `Info`).

The count needs the full history, so the workflows check out with `fetch-depth: 0`. Older releases are named `SNAPSHOT-<8 character sha>`.

## CI triggers

| Tag pushed | Workflow | Result |
|---|---|---|
| `ci-linux` | `linux.yaml` | Builds the six binaries (Linux, Darwin, Windows × `amd64`, `arm64`) and publishes a **prerelease** `v<BASE>.<n>` |
| `ci-release` | `linux.yaml` | Same build; **promotes** the prerelease of the same commit if there is one (binaries refreshed, marked latest), otherwise creates the release |
| `ci-build` | `release.yaml` | Builds the Swift agent and the macOS package, publishes a prerelease |
| `v*` | `release.yaml` | Release with the signed, notarized macOS `.pkg`; the embedded version is the tag's name |

`ci-linux` then `ci-release` on the same commit share one version: the second reuses the existing release instead of failing. If the `v…` tag already exists on another commit, publishing is refused.

Published binaries are named `cakeagent-<os>-<arch>` (with `.exe` for Windows): `cakeagent-linux-amd64`, `cakeagent-linux-arm64`, `cakeagent-darwin-amd64`, `cakeagent-darwin-arm64`, `cakeagent-windows-amd64.exe`, `cakeagent-windows-arm64.exe`. The Linux binaries embed the seamless compositor, built first by `seamlessd.yaml`.

## How Caker picks its version

Caker pins the agent version it installs in the `CAKEAGENT_VERSION` constant (`Sources/cakedlib/CloudImage/CloudInit.swift`), which is **the full release name**:

```
https://github.com/Fred78290/cakeagent/releases/download/<CAKEAGENT_VERSION>/cakeagent-<os>-<arch>
```

To get a new agent into guests:

1. merge the change in cakeagent, push `ci-linux` (or `ci-release`) and wait for the workflow to finish;
2. in Caker, set `CAKEAGENT_VERSION` to the new version (`v1.0.310`, for example).

Caker compares `CAKEAGENT_VERSION` with the `agentVersion` each guest reports, **exactly**: a guest whose agent differs shows "Update agent", and the SSH or cloud-init install replaces the agent. The exact comparison avoids taking `v1.0.12` for installed on a `v1.0.123` guest.

</div>
