<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

Bienvenue dans le wiki de **cakeagent**.

cakeagent est un agent gRPC léger qui tourne **à l'intérieur d'une machine virtuelle** et expose à l'hôte les opérations de gestion de l'invité : télémétrie système, exécution de commandes, shell interactif, montage VirtioFS, redimensionnement du disque, arrêt propre, tunnels TCP/UDP et fenêtres transparentes (seamless) pour les invités Linux.

C'est le pendant côté invité du projet [Caker](https://github.com/Fred78290/caker), qui l'installe dans les VM qu'il gère et le pilote par gRPC. Le contrat gRPC est cependant générique : tout client hôte peut l'utiliser, et un client de référence (`testagent`) est fourni.

## Liens rapides

- [Démarrage](getting-started) : compiler, installer comme service, certificats
- [Architecture](architecture) : composants, transports, deux implémentations
- [Référence CLI](cli-reference) : toutes les commandes et options
- [API gRPC](grpc-api) : les RPC, l'exécution sous un autre utilisateur
- [Fenêtres seamless](seamless-windows) : les fenêtres de l'invité Linux sur l'hôte
- [Invités Windows](windows-guests) : service, vsock, taille d'affichage
- [Versions et publication](releases) : schéma de version, étiquettes CI, lien avec Caker
- [Développement](development) : build, protobuf, tests, CI
- [Dépannage](troubleshooting)

Licence : GNU AGPL v3.

</div>

<div class="lang-en" style="display:block" markdown="1">

Welcome to the **cakeagent** wiki.

cakeagent is a lightweight gRPC agent that runs **inside a virtual machine** and exposes guest-management operations to the host: system telemetry, command execution, an interactive shell, VirtioFS mounting, disk resizing, graceful shutdown, TCP/UDP tunnels and seamless windows for Linux guests.

It is the guest-side counterpart of [Caker](https://github.com/Fred78290/caker), which installs it into the VMs it manages and drives it over gRPC. The gRPC contract is generic though: any host-side client can use it, and a reference client (`testagent`) is included.

## Quick links

- [Getting started](getting-started): build, install as a service, certificates
- [Architecture](architecture): components, transports, two implementations
- [CLI reference](cli-reference): every command and flag
- [gRPC API](grpc-api): the RPCs, running as another user
- [Seamless windows](seamless-windows): the Linux guest's windows on the host
- [Windows guests](windows-guests): service, vsock, display size
- [Versions and releases](releases): version scheme, CI tags, how Caker picks a version
- [Development](development): build, protobuf, tests, CI
- [Troubleshooting](troubleshooting)

License: GNU AGPL v3.

</div>
