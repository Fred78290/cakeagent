<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# API gRPC

Le contrat est `proto/agent.proto`, service `CakeAgentService`. Les stubs Go et Swift se régénèrent avec `proto/generate.sh` (voir [Développement](development)).

| RPC | Type | Rôle |
|---|---|---|
| `Ping` | unaire | Test de connectivité, latence aller-retour |
| `Info` | unaire | Mémoire, disques, CPU par cœur, réseau, système et hôte, nombre de processus, version de l'agent |
| `CurrentUsage` | flux serveur | Usage CPU et mémoire en direct, à la fréquence demandée |
| `Run` | unaire | Exécute une commande et renvoie code de sortie, stdout et stderr une fois terminée |
| `Execute` | flux bidirectionnel | Shell interactif ou commande, avec entrée et sortie en flux et terminal redimensionnable ; peut s'exécuter sous un autre utilisateur de l'invité |
| `Mount` / `Umount` | unaire | Monte ou démonte des partages VirtioFS (nom, cible, uid/gid, lecture seule, montage précoce) |
| `ResizeDisk` | unaire | Agrandit le disque de l'invité à tout l'espace disponible |
| `Shutdown` | unaire | Éteint proprement l'invité |
| `Tunnel` | flux bidirectionnel | Tunnel TCP ou UDP entre le réseau de l'hôte et celui de l'invité |
| `Events` | flux serveur | Ajouts et suppressions de ports redirigés par les tunnels |

## `Execute`

C'est un flux dans les deux sens. Le client envoie d'abord un `ExecuteCommand` (soit `command` avec ses arguments, soit `shell`), puis des `input`, éventuellement une `TerminalSize` (lignes et colonnes) quand le terminal est redimensionné, et `eof` pour fermer l'entrée. L'agent répond par un message `established` (succès ou raison de l'échec), puis `stdout` et `stderr`, et enfin `exitCode`. Avec une taille de terminal la commande tourne dans un pseudo-terminal, donc stderr est fusionné dans stdout ; sans elle, elle reçoit de simples tubes.

Sous Windows, le shell est PowerShell 7 (`pwsh.exe`) s'il est installé, sinon Windows PowerShell, sinon `cmd.exe` ; la taille de terminal passe par un pseudo-console (ConPTY, Windows 10 1809 ou plus).

### Exécuter sous un autre utilisateur

`command` et `shell` portent chacun un champ `user`.

- **Agent Go sous Linux** : lancé en root, l'agent démarre le processus sous ce compte, avec ses uid, gid et groupes supplémentaires, `HOME`, `USER`, `LOGNAME`, `SHELL`, `XDG_RUNTIME_DIR` et la propriété du terminal. Quand le compositeur de fenêtres tourne pour cet utilisateur, le processus reçoit aussi l'environnement du compositeur : un programme graphique lancé depuis ce shell s'ouvre sur l'hôte (voir [Fenêtres seamless](seamless-windows)). Si l'agent n'est pas root, ou si le compte n'existe pas, la requête échoue avec une raison explicite.
- **Agent Go compilé pour Darwin** : même principe, le compte est résolu par `dscl` et `id` (le service d'annuaire, politique de recherche comprise) avec un délai de 10 secondes ; seuls 16 groupes supplémentaires sont transmis (limite de `setgroups` sur macOS) ; l'environnement est filtré (`PATH`, `TERM`, `COLORTERM`, `TZ`, `LANG`, `LC_*`) puis complété de `HOME`, `USER`, `LOGNAME` et `SHELL`.
- **Un `user` vide** garde le compte de l'agent. L'agent Swift de macOS et l'agent Windows ignorent le champ.

Côté client, `exec` et `shell` de `CakeAgentHelper` (dans `CakeAgentLib`) et le flux d'exécution prennent un `user` optionnel.

## Tunnels et événements

`Tunnel` transporte des messages `TunnelMessage` : un `connect` (identifiant, protocole TCP ou UDP, adresse de l'invité), des `datas`, `eof` ou `error`. `Events` annonce les ports que l'invité ouvre ou ferme (`addedPorts`, `removedPorts`), ce qui permet à l'hôte de maintenir ses redirections de ports à jour.

</div>

<div class="lang-en" style="display:block" markdown="1">

# gRPC API

The contract is `proto/agent.proto`, service `CakeAgentService`. The Go and Swift stubs are regenerated with `proto/generate.sh` (see [Development](development)).

| RPC | Kind | Purpose |
|---|---|---|
| `Ping` | unary | Connectivity check, round-trip latency |
| `Info` | unary | Memory, disks, per-core CPU, network, OS and host, process count, agent version |
| `CurrentUsage` | server streaming | Live CPU and memory usage at the requested frequency |
| `Run` | unary | Run a command, return exit code, stdout and stderr once it completes |
| `Execute` | bidirectional streaming | Interactive shell or command, with streamed input and output and a resizable terminal; can run as another guest user |
| `Mount` / `Umount` | unary | Mount or unmount VirtioFS shares (name, target, uid/gid, read-only, early mount) |
| `ResizeDisk` | unary | Grow the guest disk to use all available space |
| `Shutdown` | unary | Gracefully power the guest off |
| `Tunnel` | bidirectional streaming | TCP or UDP tunnel between the host's network and the guest's |
| `Events` | server streaming | Port-forward additions and removals made by tunnels |

## `Execute`

It is a stream in both directions. The client first sends an `ExecuteCommand` (either `command` with its arguments, or `shell`), then `input`, possibly a `TerminalSize` (rows and columns) when the terminal is resized, and `eof` to close the input. The agent answers with an `established` message (success or the reason it failed), then `stdout` and `stderr`, and finally `exitCode`. With a terminal size the command runs under a pseudo terminal, so stderr is merged into stdout; without one it gets plain pipes.

On Windows the shell is PowerShell 7 (`pwsh.exe`) when installed, Windows PowerShell otherwise, and `cmd.exe` as a last resort; the terminal size goes through a pseudo console (ConPTY, Windows 10 1809 or later).

### Running as another user

`command` and `shell` each carry a `user` field.

- **Go agent on Linux**: running as root, the agent starts the process as that account, with its uid, gid and supplementary groups, `HOME`, `USER`, `LOGNAME`, `SHELL`, `XDG_RUNTIME_DIR` and ownership of the terminal. When the windows compositor runs for that user the process also gets the compositor's environment: a graphical program started from that shell opens on the host (see [Seamless windows](seamless-windows)). If the agent is not root, or the account does not exist, the request fails with an explicit reason.
- **Go agent built for Darwin**: same idea, but the account is resolved by `dscl` and `id` (the directory service, search policy included) with a 10 second timeout; only 16 supplementary groups are passed (the `setgroups` limit on macOS); the environment is filtered (`PATH`, `TERM`, `COLORTERM`, `TZ`, `LANG`, `LC_*`) and completed with `HOME`, `USER`, `LOGNAME` and `SHELL`.
- **An empty `user`** keeps the agent's own account. The Swift macOS agent and the Windows agent ignore the field.

On the client side, `CakeAgentHelper`'s `exec` and `shell` (in `CakeAgentLib`) and the execute stream take an optional `user`.

## Tunnels and events

`Tunnel` carries `TunnelMessage`s: a `connect` (id, TCP or UDP protocol, guest address), `datas`, `eof` or `error`. `Events` announces the ports the guest opens or closes (`addedPorts`, `removedPorts`), which lets the host keep its port forwards up to date.

</div>
