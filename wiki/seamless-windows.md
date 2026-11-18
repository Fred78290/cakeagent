<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Fenêtres seamless (invités Linux)

L'agent Linux embarque et supervise `caker-seamlessd`, un compositeur Wayland sans écran qui envoie chaque fenêtre de l'invité à l'hôte macOS par vsock. Caker affiche chacune comme une fenêtre native (titre préfixé du nom de la VM). Le compositeur est un binaire statique intégré à l'agent (construit par `scripts/build-seamlessd.sh`, voir `.github/workflows/seamlessd.yaml`, depuis les sources de Caker) : l'invité n'a besoin de rien d'autre que l'agent. Les données XKB dont le compositeur tire sa disposition de clavier sont intégrées aussi et installées à côté (`/usr/libexec/caker/xkb`) ; elles ne servent que si l'invité n'a pas son propre `xkb-data` ou `xkeyboard-config`.

## Activer

```bash
# à l'installation du service : compositeur lancé pour le premier utilisateur normal (ou --seamless-user)
cakeagent service install --seamless --seamless-layout=fr ...
```

Avec Caker, c'est `caked build --seamless` pour une nouvelle VM, ou `caked configure <vm> --seamless true` puis une mise à jour de l'agent pour une VM existante.

`--seamless-layout` doit correspondre à la disposition du clavier de l'hôte : les touches voyagent par position.

## Sous-commandes `cakeagent seamless`

```bash
cakeagent seamless install                      # écrit le compositeur embarqué dans /usr/libexec/caker/
cakeagent seamless run                          # superviseur au premier plan : le lance en tant qu'utilisateur et le relance
cakeagent seamless status [--json]              # installé ? en marche ? code de sortie 1 sinon
cakeagent seamless apps [--json]                # applications .desktop de l'invité
cakeagent seamless launch --desktop org.gnome.gedit
cakeagent seamless launch -- foot               # n'importe quel programme, en tant qu'utilisateur, avec WAYLAND_DISPLAY & co.
cakeagent seamless env                          # l'environnement dont les applications ont besoin
```

Un build de développement sans compositeur embarqué installe celui que désigne `$CAKER_SEAMLESSD`, ou celui trouvé dans le `PATH`.

## Utilisateur, socket et plusieurs utilisateurs

L'agent tourne en root, mais le compositeur et les applications tournent sous l'utilisateur : les clients Wayland, le D-Bus et l'audio vivent dans le répertoire d'exécution de l'utilisateur (`loginctl enable-linger` fait exister `/run/user/<uid>` sans ouverture de session). Le socket Wayland est `wayland-1` : cette forme standard est la seule que le profil AppArmor de snap laisse ouvrir aux applications confinées (Firefox sur Ubuntu), et `wayland-0` est généralement réservé à une session de bureau.

Chaque utilisateur de l'invité peut avoir son propre compositeur. L'hôte n'accepte qu'une connexion par VM : l'agent s'intercale donc avec un **hub**.

```
caker-seamlessd (alice) --\
caker-seamlessd (bob)   ---+--> hub (agent) --> une connexion vsock --> l'hôte
```

- Chaque compositeur se connecte au socket unix du hub (`/run/caker-seamless/hub.sock`) ; le noyau dit qui il est (`SO_PEERCRED`), et root est refusé.
- Le hub fait l'échange de salutation avec l'hôte une seule fois et met le numéro d'emplacement du compositeur dans l'octet de poids fort de chaque identifiant de fenêtre ; l'entrée renvoyée par l'hôte est routée par cet octet, et le clavier va au compositeur qui possède la fenêtre ayant le focus.
- Les titres de fenêtre reçoivent le préfixe `<utilisateur> : `.
- Quand un compositeur disparaît, le hub détruit ses fenêtres côté hôte sans toucher à celles des autres.
- Le hub borne la mémoire des images en vol, la file d'écriture de chaque compositeur et les délais d'écriture vers l'hôte ; il sert jusqu'à 255 compositeurs.
- Un compositeur est démarré pour un utilisateur quand quelque chose en demande un, par exemple une commande `Execute` ou `cakeagent seamless launch` sous ce compte.

Rien ne change au format du protocole : un hôte et un compositeur qui ignorent le hub continuent de fonctionner. Le protocole est dupliqué à la main dans le compositeur (`Tools/SeamlessCompositor/wire.{h,c}` de Caker) et dans `linux/pkg/seamless` : toute modification doit toucher les deux, et le côté hôte de Caker.

</div>

<div class="lang-en" style="display:block" markdown="1">

# Seamless windows (Linux guests)

The Linux agent embeds and supervises `caker-seamlessd`, a headless Wayland compositor that streams every window of the guest to the macOS host over vsock. Caker shows each one as a native window (titles prefixed with the VM's name). The compositor is a static binary embedded in the agent (built by `scripts/build-seamlessd.sh`, see `.github/workflows/seamlessd.yaml`, from Caker's sources), so the guest needs nothing but the agent. The XKB data the compositor builds its keymap from is embedded too and installed beside it (`/usr/libexec/caker/xkb`); it is only used when the guest has no `xkb-data` or `xkeyboard-config` of its own.

## Turning it on

```bash
# when installing the service: compositor run for the first regular user (or --seamless-user)
cakeagent service install --seamless --seamless-layout=fr ...
```

With Caker that is `caked build --seamless` for a new VM, or `caked configure <vm> --seamless true` followed by an agent update for an existing one.

`--seamless-layout` must match the host's keyboard layout: keys travel by position.

## `cakeagent seamless` subcommands

```bash
cakeagent seamless install                      # write the embedded compositor to /usr/libexec/caker/
cakeagent seamless run                          # foreground supervisor: starts it as the user, restarts it
cakeagent seamless status [--json]              # installed? running? exit status 1 when it is not
cakeagent seamless apps [--json]                # .desktop applications of the guest
cakeagent seamless launch --desktop org.gnome.gedit
cakeagent seamless launch -- foot               # any program, as the user, with WAYLAND_DISPLAY & co.
cakeagent seamless env                          # the environment applications need
```

A development build without the embedded compositor installs the one named by `$CAKER_SEAMLESSD`, or found on `PATH`.

## User, socket and several users

The agent runs as root, but the compositor and the applications run as the user: Wayland clients, D-Bus and audio live in the user's runtime directory (`loginctl enable-linger` makes `/run/user/<uid>` exist without a login). The Wayland socket is `wayland-1`: that standard form is the only one snap's AppArmor profile lets confined applications open (Firefox on Ubuntu), and `wayland-0` is typically reserved for a desktop session.

Each guest user can have a compositor of their own. The host accepts one connection per VM, so the agent sits in between with a **hub**.

```
caker-seamlessd (alice) --\
caker-seamlessd (bob)   ---+--> hub (agent) --> one vsock connection --> the host
```

- Each compositor connects to the hub's unix socket (`/run/caker-seamless/hub.sock`); the kernel says who it is (`SO_PEERCRED`), and root is refused.
- The hub does the hello exchange with the host once and puts the compositor's slot in the top byte of every window id; the input the host sends back is routed by that byte, and the keyboard goes to the compositor that owns the focused window.
- Window titles get a `<user>: ` prefix.
- When a compositor goes away the hub destroys its windows on the host without touching the others'.
- The hub bounds the memory of frames in flight, each compositor's write queue and the write deadlines to the host; it serves up to 255 compositors.
- A compositor is started for a user when something asks for one, for example an `Execute` command or `cakeagent seamless launch` as that account.

Nothing changes in the wire format: a host and a compositor that know nothing about the hub keep working. The protocol is mirrored by hand in the compositor (Caker's `Tools/SeamlessCompositor/wire.{h,c}`) and in `linux/pkg/seamless`: change both, and Caker's host side, together.

</div>
