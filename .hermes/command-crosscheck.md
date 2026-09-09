# Saturn → Zenbot command cross-check

Generated from Saturn `origin/develop` command annotations and Zenbot’s static catalog/dispatch code. `PLACEHOLDER_OR_EXTERNAL` requires source-level review; it is not migration acceptance.

- Source command classes: **64**
- Source aliases: **147**
- Catalog alias mismatches: **7**
- Concrete dispatch cases: **40**
- Placeholder/external cases: **24**

| Source class | Canonical | Saturn aliases | Catalog aliases | Dispatch | Alias gap |
|---|---|---|---|---|---|
| `AccessUserCommandImpl` | `grant` | `grant`, `access` |  | PLACEHOLDER_OR_EXTERNAL | access, grant |
| `MemoryCommandImpl` | `mem` | `mem`, `memory`, `memstats` |  | PLACEHOLDER_OR_EXTERNAL | mem, memory, memstats |
| `MineTripCommandImpl` | `mine` | `mine` | `mine` | PLACEHOLDER_OR_EXTERNAL | — |
| `PrefixCommandImpl` | `prefix` | `prefix` | `prefix` | PLACEHOLDER_OR_EXTERNAL | — |
| `ReplicaCommandImpl` | `replica` | `replica`, `bot`, `agent` | `replica`, `bot`, `agent` | PLACEHOLDER_OR_EXTERNAL | — |
| `ReplicaOffCommandImpl` | `replicaoff` | `replicaoff`, `offline`, `botoff`, `agentoff` | `replicaoff`, `offline`, `botoff`, `agentoff` | PLACEHOLDER_OR_EXTERNAL | — |
| `ReplicaStatusCommandImpl` | `replicastatus` | `replicastatus`, `status` | `replicastatus`, `status` | PLACEHOLDER_OR_EXTERNAL | — |
| `RestartCommandImpl` | `restart` | `restart`, `reload`, `re` | `restart`, `reload`, `re` | PLACEHOLDER_OR_EXTERNAL | — |
| `ShutdownCommandImpl` | `exit` | `exit`, `quit`, `shutdown` |  | PLACEHOLDER_OR_EXTERNAL | exit, quit, shutdown |
| `SqlUserCommandImpl` | `sql` | `sql` | `sql` | PLACEHOLDER_OR_EXTERNAL | — |
| `WhiskeyReplicaCommandImpl` | `whiskey` | `whiskey` | `whiskey` | PLACEHOLDER_OR_EXTERNAL | — |
| `DBZAddStrCommandImpl` | `dbzstr` | `dbzstr`, `dstr`, `daddstr` | `dbzstr`, `dstr`, `daddstr` | CONCRETE | — |
| `DBZFightCommandImpl` | `dfight` | `dfight`, `df` | `dfight`, `df` | CONCRETE | — |
| `DBZHelpCommandImpl` | `dbzhelp` | `dbzhelp`, `dbz`, `dhelp` | `dbzhelp`, `dbz`, `dhelp` | CONCRETE | — |
| `DBZRegisterCommandImpl` | `dbzregister` | `dbzregister`, `dreg`, `dr` | `dbzregister`, `dreg`, `dr` | CONCRETE | — |
| `DBZSpawnEnemyCommandImpl` | `dspawn` | `dspawn` | `dspawn` | CONCRETE | — |
| `DBZStatsCommandImpl` | `dbzstats` | `dbzstats`, `dstats`, `dstat`, `ds` | `dbzstats`, `dstats`, `dstat`, `ds` | CONCRETE | — |
| `ActivityCommandImpl` | `active` | `active`, `activity` | `active`, `activity` | PLACEHOLDER_OR_EXTERNAL | — |
| `AuthorizeTripCommandImpl` | `authorize` | `authorize`, `auth` | `authorize`, `auth` | CONCRETE | — |
| `AutoMoveUserCommandImpl` | `automove` | `automove` | `automove` | PLACEHOLDER_OR_EXTERNAL | — |
| `BanUserCommandImpl` | `ban` | `ban` | `ban` | CONCRETE | — |
| `CaptchaCommandImpl` | `captcha` | `captcha` | `captcha` | CONCRETE | — |
| `ColorCommandImpl` | `color` | `color` | `color` | CONCRETE | — |
| `DeAuthorizeTripCommandImpl` | `deauthorize` | `deauthorize`, `deauth` | `deauthorize`, `deauth` | CONCRETE | — |
| `FlairCommandImpl` | `flair` | `flair` | `flair` | CONCRETE | — |
| `KickUserCommandImpl` | `kick` | `kick`, `k`, `out` | `kick`, `k`, `out` | CONCRETE | — |
| `LastMessagesCommandImpl` | `messages` | `messages`, `lastmessages` | `messages`, `lastmessages` | CONCRETE | — |
| `LockRoomUserCommandImpl` | `lock` | `lock`, `lockroom` | `lock`, `lockroom` | CONCRETE | — |
| `MuteUserCommandImpl` | `mute` | `mute`, `dumb` | `mute`, `dumb` | CONCRETE | — |
| `NukeCommandImpl` | `nuke` | `nuke` | `nuke` | PLACEHOLDER_OR_EXTERNAL | — |
| `OverflowCommandImpl` | `overflow` | `overflow`, `shoot`, `love`, `hug`, `kiss` | `overflow`, `shoot`, `love`, `hug`, `kiss` | CONCRETE | — |
| `RegisterUserCommandImpl` | `reg` | `reg`, `register` |  | PLACEHOLDER_OR_EXTERNAL | reg, register |
| `RemoveUserCommandImpl` | `del` | `del`, `delete`, `remove` |  | PLACEHOLDER_OR_EXTERNAL | del, delete, remove |
| `ResurrectUserCommandImpl` | `move` | `move`, `recover`, `heal`, `resurrect` |  | PLACEHOLDER_OR_EXTERNAL | heal, move, recover, resurrect |
| `ShadowBanList` | `shadowbanlist` | `shadowbanlist`, `banlist`, `bannedusers` | `shadowbanlist`, `banlist`, `bannedusers` | CONCRETE | — |
| `ShadowBanUserCommandImpl` | `shadowban` | `shadowban`, `sban` | `shadowban`, `sban` | CONCRETE | — |
| `UnBanAllUserCommandImpl` | `unbanall` | `unbanall`, `pardonall` | `unbanall`, `pardonall` | CONCRETE | — |
| `UnBanUserCommandImpl` | `unban` | `unban` | `unban` | CONCRETE | — |
| `UnMuteUserCommandImpl` | `unmute` | `unmute`, `undumb` | `unmute`, `undumb` | CONCRETE | — |
| `UnShadowBanUserCommandImpl` | `unshadowban` | `unshadowban`, `shadowmercy`, `unblock` | `unshadowban`, `shadowmercy`, `unblock` | CONCRETE | — |
| `AfkUserCommandImpl` | `afk` | `afk`, `a` | `afk`, `a` | CONCRETE | — |
| `ApeUserCommandImpl` | `ape` | `ape`, `harambe` | `ape`, `harambe` | CONCRETE | — |
| `CoinUserCommandImpl` | `coin` | `coin`, `toss`, `ct` | `coin`, `toss`, `ct` | CONCRETE | — |
| `HelpUserCommandImpl` | `help` | `help`, `h` | `help`, `h` | CONCRETE | — |
| `HowToUserCommandImpl` | `crashcourse` | `crashcourse`, `howto`, `moderationcrashcourse`, `hcguide` |  | PLACEHOLDER_OR_EXTERNAL | crashcourse, hcguide, howto, moderationcrashcourse |
| `InfoUserCommandImpl` | `info` | `info`, `i`, `whois`, `who` | `info`, `i`, `whois`, `who` | CONCRETE | — |
| `LUserCommandImpl` | `l` | `l` | `l` | PLACEHOLDER_OR_EXTERNAL | — |
| `LastOnlineUserCommandImpl` | `lastonline` | `lastonline`, `seen`, `last`, `online`, `lastseen` | `lastonline`, `seen`, `last`, `online`, `lastseen` | CONCRETE | — |
| `ListNicksCommandImpl` | `nicks` | `nicks`, `t2n` | `nicks`, `t2n` | CONCRETE | — |
| `ListUserCommandImpl` | `list` | `list` | `list` | CONCRETE | — |
| `MailUserCommandImpl` | `mail` | `mail`, `msg`, `send` | `mail`, `msg`, `send` | CONCRETE | — |
| `MsgChannelCommandImpl` | `msgchannel` | `msgchannel`, `msgroom` | `msgchannel`, `msgroom` | PLACEHOLDER_OR_EXTERNAL | — |
| `NoteUserCommandImpl` | `note` | `note`, `save` | `note`, `save` | CONCRETE | — |
| `NotesUserCommandImpl` | `notes` | `notes` | `notes` | CONCRETE | — |
| `PingUserCommandImpl` | `ping` | `ping`, `p` | `ping`, `p` | CONCRETE | — |
| `PrintNickTripUserCommandImpl` | `users` | `users`, `whitelist`, `blacklist`, `offenders`, `knownoffenders` | `users`, `whitelist`, `blacklist`, `offenders`, `knownoffenders` | CONCRETE | — |
| `SayUserCommandImpl` | `say` | `say`, `echo` | `say`, `echo` | CONCRETE | — |
| `SubscribeUserCommandImpl` | `sub` | `sub`, `subscribe` | `sub`, `subscribe` | PLACEHOLDER_OR_EXTERNAL | — |
| `TimeUserCommandImpl` | `time` | `time`, `t` | `time`, `t` | CONCRETE | — |
| `UnsubscribeUserCommandImpl` | `unsub` | `unsub`, `unsubscribe` | `unsub`, `unsubscribe` | PLACEHOLDER_OR_EXTERNAL | — |
| `VersionUserCommandImpl` | `version` | `version`, `v` | `version`, `v` | CONCRETE | — |
| `WeatherUserCommandImpl` | `weather` | `weather`, `w`, `today` | `weather`, `w`, `today` | CONCRETE | — |
| `WhiskeyAnonUserCommandImpl` | `wsa` | `wsa`, `wsayanon`, `anonsay` | `wsa`, `wsayanon`, `anonsay` | PLACEHOLDER_OR_EXTERNAL | — |
| `WhiskeySayUserCommandImpl` | `ws` | `ws`, `wsay` | `ws`, `wsay` | PLACEHOLDER_OR_EXTERNAL | — |
