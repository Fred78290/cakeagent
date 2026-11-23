<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Dépannage

## Savoir ce qui tourne dans l'invité

```bash
cakeagent version          # « The current version is: v1.0.310, build at: … »
cakeagent infos --json     # version de l'agent, système, réseau, disques (lus sur la machine locale)
cakeagent --log-level=debug serve --listen=tcp://127.0.0.1:5010
```

`v0.0.0-unset` signifie un binaire compilé sans l'option d'édition de liens de version (voir [Démarrage](getting-started)). Un agent dont la version diffère de celle de Caker est affiché « Update agent » dans Caker.

## L'hôte ne joint pas l'agent

- Dans un invité, l'agent écoute en `vsock://any:5000` par défaut : la VM doit avoir un périphérique vsock, et le service doit tourner (`systemctl status cakeagent` sous systemd).
- Avec TLS, le certificat serveur doit être signé par la CA que l'hôte connaît ; une erreur de handshake vient presque toujours d'une CA ou d'un certificat qui ne correspondent pas.
- Sous Windows, l'adresse par défaut est `tcp://127.0.0.1:5000`, que l'hôte ne peut pas joindre : utilisez `vsock://any:5000` (voir [Invités Windows](windows-guests)).

## SELinux (RHEL, Fedora, CentOS et dérivés)

`cakeagent service install` détecte SELinux et étiquette le binaire installé `bin_t` avec `semanage fcontext` puis `restorecon`, ce qui suffit normalement à ce que le gestionnaire de services l'exécute. Cela demande `semanage` (paquet `policycoreutils-python-utils`) : sans lui, l'agent ne peut pas s'étiqueter lui-même. Si le service ne démarre toujours pas sur un hôte en mode `enforcing`, `install` journalise un avertissement avec les commandes ci-dessous. Lancez-les vous-même plutôt que de les scripter : `audit2allow` génère une politique qui accorde **tout** ce qu'il trouve refusé, à relire avant de la charger.

```bash
# voir ce qui a été refusé
ausearch -m avc -c cakeagent

# si les refus sont attendus : générer, relire, puis charger une politique
ausearch -m avc -c 'cakeagent' --raw | audit2allow -M my-cakeagent
cat my-cakeagent.te   # à relire avant de charger
semodule -i my-cakeagent.pp
restorecon -v /usr/local/bin/cakeagent
```

## Les fenêtres seamless n'apparaissent pas

- `cakeagent seamless status` : le compositeur est-il installé et en marche ? (code de sortie 1 sinon).
- `cakeagent seamless version` : version du compositeur embarqué dans l'agent, de celui qui est installé et de ceux qui tournent (un par utilisateur). Un compositeur en marche plus ancien que celui qui est installé (agent mis à jour sur place) est relancé par `systemctl restart cakeagent`.
- La disposition `--seamless-layout` doit correspondre au clavier de l'hôte, sinon les touches tapent autre chose.
- Les applications doivent être lancées avec l'environnement du compositeur : `cakeagent seamless launch -- <programme>`, ou un shell ouvert par l'hôte sous l'utilisateur seamless.
- Un hôte plus ancien ne connaît pas le défilement à la molette : la version du protocole est négociée à la baisse.

## Windows : l'agent plante au démarrage

Sur certaines machines (Windows Server 2022 et 2025 x64, et des invités Windows 11 x64 sur certains Mac Intel), le runtime Go de l'agent s'arrêtait avec `fatal error: runtime: mcall called on m->g0 stack` quelques secondes après le démarrage, avant d'avoir répondu au SCM (événement 7009 : délai d'attente de la connexion du service). La parade est de lancer l'agent avec `GODEBUG=asyncpreemptoff=1` dans l'environnement du service, ce que l'installateur de Caker fait pour tous les invités x64 ; les invités Windows Server ont en plus un démarrage différé et réessayé. Si vous installez l'agent à la main sur un tel invité, définissez cette variable sur le service.

</div>

<div class="lang-en" style="display:block" markdown="1">

# Troubleshooting

## Finding out what runs in the guest

```bash
cakeagent version          # "The current version is: v1.0.310, build at: …"
cakeagent infos --json     # agent version, system, network, disks (read on the local machine)
cakeagent --log-level=debug serve --listen=tcp://127.0.0.1:5010
```

`v0.0.0-unset` means a binary built without the version link flag (see [Getting started](getting-started)). An agent whose version differs from Caker's is shown as "Update agent" in Caker.

## The host cannot reach the agent

- In a guest the agent listens on `vsock://any:5000` by default: the VM needs a vsock device, and the service must be running (`systemctl status cakeagent` under systemd).
- With TLS the server certificate must be signed by the CA the host knows; a handshake error almost always comes from a CA or certificate that don't match.
- On Windows the default address is `tcp://127.0.0.1:5000`, which the host cannot reach: use `vsock://any:5000` (see [Windows guests](windows-guests)).

## SELinux (RHEL, Fedora, CentOS and derivatives)

`cakeagent service install` detects SELinux and labels the installed binary `bin_t` with `semanage fcontext` then `restorecon`, which normally lets the service manager run it. That needs `semanage` (package `policycoreutils-python-utils`): without it the agent cannot label itself. If the service still fails to start on an enforcing host, `install` logs a warning with the commands below. Run them yourself rather than scripting them: `audit2allow` generates a policy that grants **everything** it finds denied, to be reviewed before loading.

```bash
# see what was actually denied
ausearch -m avc -c cakeagent

# if the denials are expected, generate, review, then load a policy for them
ausearch -m avc -c 'cakeagent' --raw | audit2allow -M my-cakeagent
cat my-cakeagent.te   # review before loading
semodule -i my-cakeagent.pp
restorecon -v /usr/local/bin/cakeagent
```

## Seamless windows do not appear

- `cakeagent seamless status`: is the compositor installed and running? (exit status 1 when it is not).
- `cakeagent seamless version`: the version of the compositor the agent carries, of the installed one and of the ones running (one per user). A running compositor older than the installed one (agent updated in place) is restarted by `systemctl restart cakeagent`.
- `--seamless-layout` must match the host's keyboard, otherwise keys type something else.
- Applications must be started with the compositor's environment: `cakeagent seamless launch -- <program>`, or a shell the host opens as the seamless user.
- An older host does not know mouse wheel scrolling: the protocol version is negotiated down.

## Windows: the agent crashes at start

On some machines (Windows Server 2022 and 2025 x64, and Windows 11 x64 guests on some Intel Macs) the agent's Go runtime stopped with `fatal error: runtime: mcall called on m->g0 stack` a few seconds after starting, before answering the SCM (event 7009: timeout waiting for the service to connect). The workaround is to run the agent with `GODEBUG=asyncpreemptoff=1` in the service's environment, which Caker's installer does for every x64 guest; Windows Server guests also get a delayed, retried start. If you install the agent by hand on such a guest, set that variable on the service.

</div>
