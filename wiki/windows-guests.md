<!-- markdownlint-disable MD033 MD024 -->

<div class="lang-fr" style="display:none" markdown="1">

# Invités Windows

L'agent Go se compile pour `windows/{amd64,arm64}` (`cakeagent-windows-<arch>.exe`). Caker l'installe lui-même dans un invité Windows ; cette page décrit ce que fait l'agent.

## Service

`cakeagent service install` enregistre un service que le gestionnaire de services (SCM) démarre sous la forme `cakeagent serve …`. `serve` répond au SCM quand il est lancé ainsi, et tourne au premier plan quand on le lance depuis une console.

## Transports

L'adresse par défaut est `tcp://127.0.0.1:5000`. Pour être joignable depuis l'hôte, écoutez sur `vsock://any:5000` : cela demande le pilote `viosock` de virtio-win dans l'invité (l'agent parle à son fournisseur Winsock ; la famille d'adresses est lue auprès du pilote, 40 par défaut) et un périphérique vsock sur la VM. `virtio://` et `unix://` ne sont pas pris en charge, ni `mount` et `umount`.

## Exécution

`Execute` démarre PowerShell 7 (`pwsh.exe`) s'il est installé, Windows PowerShell sinon, et `cmd.exe` en dernier recours. Avec une taille de terminal, la commande tourne dans un pseudo-console (ConPTY, Windows 10 1809 ou plus), donc stderr est fusionné dans stdout comme sur un pty Unix ; sans, elle reçoit de simples tubes. Le champ `user` de la requête est ignoré.

## Taille d'affichage

Quand `HKLM\SOFTWARE\Caker\FollowDisplaySize` vaut `1` (l'installateur de Caker le positionne quand l'invité a le pilote GPU virtio de Caker), le service garde dans la session de l'utilisateur connecté un assistant caché `cakeagent display-watch`, qui tourne sous cet utilisateur : seul un processus de la session de l'utilisateur peut interroger et changer l'affichage.

- L'assistant trouve le GPU virtio dans la liste des affichages, lit chaque seconde la taille que l'hôte demande par le pilote, et l'applique avec `ChangeDisplaySettingsEx` quand cette demande **change** (une résolution choisie par l'utilisateur n'est donc pas annulée).
- Une taille refusée est réessayée pendant quelques secondes, puis le mode le plus proche de la liste est utilisé.
- Il fait aussi du GPU virtio le seul affichage du bureau, pour que fenêtres et barre des tâches n'atterrissent pas sur l'affichage du framebuffer du firmware, que l'hôte ne montre pas. `HKLM\SOFTWARE\Caker\KeepBasicDisplay` à `1` le conserve.
- Il journalise dans `C:\ProgramData\caker\display.log` ; au démarrage de l'assistant, un journal de plus de 512 Kio est renommé `display.log.old`.
- Cela ne fonctionne que pendant qu'un utilisateur est connecté : l'écran de verrouillage garde la dernière taille.

La taille est aussi limitée par le tampon d'image du pilote GPU, fixé à l'installation par Caker : une taille dont la surface ne tient pas est refusée (`DISP_CHANGE_BADMODE` dans le journal).

## Voir aussi

[Dépannage](troubleshooting) pour le plantage connu du runtime Go sous certains Windows x64.

</div>

<div class="lang-en" style="display:block" markdown="1">

# Windows guests

The Go agent builds for `windows/{amd64,arm64}` (`cakeagent-windows-<arch>.exe`). Caker installs it into a Windows guest itself; this page describes what the agent does there.

## Service

`cakeagent service install` registers a service that the Service Control Manager (SCM) starts as `cakeagent serve …`. `serve` answers the SCM when it is started that way, and runs in front when started from a console.

## Transports

The default address is `tcp://127.0.0.1:5000`. To be reachable from the host, listen on `vsock://any:5000`: that needs the virtio-win `viosock` driver in the guest (the agent talks to its Winsock provider; the address family is read from the driver, 40 by default) and a vsock device on the VM. `virtio://` and `unix://` are not supported, nor are `mount` and `umount`.

## Execution

`Execute` starts PowerShell 7 (`pwsh.exe`) when installed, Windows PowerShell otherwise, and `cmd.exe` as a last resort. With a terminal size the command runs under a pseudo console (ConPTY, Windows 10 1809 or later), so stderr is merged into stdout as on a Unix pty; without one it gets plain pipes. The request's `user` field is ignored.

## Display size

When `HKLM\SOFTWARE\Caker\FollowDisplaySize` is `1` (Caker's installer sets it when the guest has Caker's virtio GPU driver), the service keeps a hidden `cakeagent display-watch` helper running in the logged-on user's session, as that user: only a process in the user's session can query and change the display.

- The helper finds the virtio GPU in the display list, reads the size the host asks for through the driver every second, and applies it with `ChangeDisplaySettingsEx` when that request **changes** (so a resolution the user picks isn't undone).
- A refused size is retried for a few seconds, then the closest listed mode is used.
- It also makes the virtio GPU the only display on the desktop, so windows and the taskbar don't land on the firmware framebuffer display that the host doesn't show. `HKLM\SOFTWARE\Caker\KeepBasicDisplay` set to `1` keeps it.
- It logs to `C:\ProgramData\caker\display.log`; when the helper starts and the log is over 512 KiB, it is renamed to `display.log.old`.
- It only works while a user is logged on: the lock screen keeps the last size.

The size is also bounded by the GPU driver's frame buffer, set when Caker installs it: a size whose surface does not fit is refused (`DISP_CHANGE_BADMODE` in the log).

## See also

[Troubleshooting](troubleshooting) for the known Go runtime crash on some x64 Windows guests.

</div>
