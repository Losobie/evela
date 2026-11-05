# Data and history reference

[User guide](../README.md) · [Developer guide](DEVELOPMENT.md) · [ESI setup](ESI.md)

## Loading and storage

Evela reads English EVE game logs in UTF-8 or BOM-marked UTF-16. The `Listener` header identifies the character. Client formatting tags are removed and text entities decoded. Every timestamped message is retained for Events, including non-combat messages. Files without a usable Listener header or with read errors are reported in **Details…**.

The default path is the Windows Documents known folder, including redirected/OneDrive Documents, under `EVE\logs\gamelogs`. **Settings → Log loading…** accepts one folder per line, including local paths, mapped drives and UNC shares. Windows must already grant read access. Evela does not modify source logs.

The default 30-day file limit uses the `YYYYMMDD_` filename prefix, measured from the current EVE date, with an inclusive cutoff. It does not use modification times or message timestamps. Older files and unrecognized filename dates are skipped before parsing. **Load all log files** includes those files. Graph **All history** does not override this loading limit.

Configured folders are scanned concurrently, on launch and every ten seconds while Auto refresh is enabled. New files copied or merged into a folder are discovered. Identical file exports count once across folders; different-content logs remain separate even when names or some events overlap. Repeated events are not otherwise deduplicated.

If a folder becomes unavailable, its last loaded data remains available subject to the age limit, and Details identifies the failed path. Slow scans get up to 15 seconds initially, then continue in the background while other folders refresh. Only one scan per folder runs at a time. Failed folders are retried; removing a folder removes its contribution immediately.

### Local files

| Location on Windows | Purpose |
| --- | --- |
| `%LOCALAPPDATA%\Evela\logs.sqlite` | Rebuildable parsed-log cache; SQLite is bundled with the app. |
| `%APPDATA%\Evela\esi.dat` | ESI settings, encrypted tokens, and recorded ship/location history. |

Unchanged files reuse cached parsed data across launches. Size or modification-time changes invalidate file entries; parser updates also trigger reprocessing. Deleted or excluded source files do not contribute simply because a payload remains cached. If the cache is unavailable or damaged, Evela falls back to source logs and reports the issue. To force a rebuild, close Evela before deleting `logs.sqlite`; retain your original logs. Do not delete `esi.dat` as part of a cache rebuild: it contains history and connections.

For implementation and versioning requirements, see [parsed-log cache maintenance](DEVELOPMENT.md#parsed-log-cache-maintenance).

## Damage and extracted fields

Numeric combat messages use `to` for outgoing damage and `from` for incoming damage. Damage exchanged between two loaded characters appears in each corresponding direction. Misses are zero-damage records with strength **Misses**, and do not increase landed-hit counts.

Damage records can include amount, target/attacker, location, corporation name/ticker, target type, weapon type, hit strength and deflected amount. Optional fields remain unknown or empty when not supplied. For example:

```text
(combat) 60 to Orbital Skyhook (R3-K7K IX) [Unfinished Symphony][MOVMT](Orbital Skyhook) - Vespa II - Glances Off - Glances Off (60 deflected)
```

| Field | Value |
| --- | --- |
| Amount | 60 |
| Target | Orbital Skyhook |
| Location | R3-K7K IX |
| Corporation | Unfinished Symphony |
| Corporation ticker | MOVMT |
| Target type | Orbital Skyhook |
| Weapon type | Vespa II |
| Weapon strength | Glances Off |
| Deflected amount | 60 |

Deflection is separate metadata: it is neither added to nor subtracted from the leading damage amount. A repeated strength before the deflection annotation does not replace the weapon name.

Explicit logged target types take priority. Without one, an exact case-insensitive match in the embedded hull catalog can supply a class. For example, **Blood Raiders Stronghold** and **Guristas Pirates Stronghold** resolve to **♦ Stronghold**. Unmatched names stay unknown; the fallback also applies to outgoing misses and logistics.

### NPC identification

NPC names match the embedded CCP Entity catalog exactly, ignoring capitalization and surrounding whitespace, after weapon/quality suffix removal. Player corporation/hull suffixes are handled first. NPC Types are CCP groups, such as Sentry Gun or Irregular Frigate, rather than guesses based on words in a name.

When multiple types share a display name, only IDs common to all candidates resolve; conflicting groups appear as **Ambiguous NPC type**. Players and unmatched names appear under **Other / unresolved**. Catalog matching works offline; a new NPC introduced after the bundled snapshot may remain unresolved. Snapshot provenance and regeneration are in the [developer guide](DEVELOPMENT.md#embedded-game-catalogs).

## Logistics, E-War and mining

Remote shield boosts, armor repairs and capacitor transfers appear in outgoing/incoming logistics. `to` is outgoing; `by` or `from` is incoming. Repairs use HP and capacitor transfers use GJ, with separate axes and series when combined. Local repairers, shield/capacitor boosters, Nosferatu and neutralizers are excluded from logistics.

Logistics entities support several layouts:

| Logged entity | Extracted fields |
| --- | --- |
| `Customs Office (R3-K7K IX) [Way So Mad]` | Target/type Customs Office; location R3-K7K IX; corporation Way So Mad |
| `Basilisk [THIRD] [.AE..] [Clento Loso]` | Target Clento Loso; type Basilisk; alliance ticker THIRD; corporation ticker .AE.. |
| `Stork [53C0N] [Alpha Arterius]` | Target Alpha Arterius; type Stork; corporation ticker 53C0N |

Single- or double-dash module separators are accepted. Zero-value repairs retain their fields and remain visible in graphs/events.

Incoming E-War shows **Effect events** or **Capacitor drained (GJ)**. It includes logged incoming neutralizer/Nosferatu losses and supported notifications for tackle, webs, painters, weapon disruption, dampening and ECM. Notifications do not prove success, and EVE does not consistently log all effects. Missing effects, durations and strengths cannot be reconstructed; an empty graph does not establish absence of E-War.

Mining keeps **Ore mined**, **Compressed** and **Waste** as separate series. Ore Name preserves exact grades; Ore Type uses CCP groups, including ice and compressed variants. Missing names can match a known ore-name suffix; unresolved entries remain **Unknown ore type**. Compression uses reported output units and the original ore grade for filtering.

## Ranges, filters and rates

Global filters intersect graph-specific filters before grouping, bucketing and averaging. Choices within one filter use OR; separate visible filters use AND. Hidden filters are inactive. Events uses only the range and global Characters/Character Group filters; Engagements uses all loaded logs.

Named character groups can overlap. A shared character contributes to each matching group series/row, while overall selected-character summaries count it once. Incoming source grouping combines the same logged attacker/corporation/hull; game logs do not supply a unique attacking ship item ID or custom ship name. Outgoing target grouping does not assign incoming damage to those targets.

Graph intervals are automatic, with up to 1,200 buckets and a one-second minimum width. With averaging off, events retain their original timestamp and amount, including zero values. The 10- and 30-second rolling averages divide a trailing window's events by its full duration, including idle seconds and available events before the displayed range. Coarse buckets show the mean of those rates. Units are damage/s, HP/s, GJ/s, events/s or mining units/s as appropriate. Totals use original records in the selected range; average DPS there includes the entire range's idle time.

Optional structure invulnerability markers require a recognized structure hit followed within 30 seconds by **Target is invulnerable.** Ships and unrecognized targets clear that context. Legacy control towers and customs offices also require a corporation ticker. Repeated warnings collapse until another structure hit, and identical reports across characters merge. Markers remain at the notification time and add no damage or hits.

## Engagement detection and summaries

A gap of at least ten minutes without damage, incoming E-War, positive shield/armor repairs or mining separates engagements. Damage includes misses and zero-damage hits. Zero repairs, capacitor transfers, travel and chat do not create, bridge or extend engagements, or contribute to their summaries.

Combat and mining outrank support activity. Between them, distinct active seconds per character determine the dominant activity, with combat weighted three times as heavily. Rapid weapon messages and mining waste duplicates do not inflate that score. Sustained mining can outweigh occasional combat; positive repairs and E-War can form engagements when combat/mining are absent.

Summaries show up to three systems, two target/attacker groups and two own-ship groups. A group with a single hull or NPC name uses that name; mixed groups use the class/type. Own ships are resolved at activity timestamps; unknown ship details are omitted.

Double-clicking opens Graphs with the actual start/end, including the final activity second, plus 10% of the duration on each side. Padding rounds up to whole seconds, with at least one second per side. Existing graph filters apply, and range undo restores the previous view.

## Ship and location history

ESI polls every 30 seconds while enabled. Consecutive successful observations of the same ship/location combine into periods. Changes, failures, gaps over 90 seconds, reconnecting or restarting begin new periods. These are observed ESI response times, not exact boarding, undocking or travel times; API caching and polling limit precision.

Event attribution uses the matching period and allows up to 45 seconds after its last observation, unless a newer observation begins. Earlier or uncovered times, or ambiguous character names, remain unknown. Stored timestamps are not extended. Attribution occurs before averaging, so an event stays assigned to its original ship/system. History survives disconnects and restarts without automatic expiry. Missing hull/class/system names can be resolved later from saved IDs without changing observed times.

**Ship history…** shows character, ship item/hull/class IDs and names, ship name, observed bounds and count. **Location history…** separates Game logs from ESI observations; ESI records include system IDs/names and station/structure IDs where available.

### Logged locations and wormholes

Explicit jump and Conduit Field messages take priority over ESI, which may still report a cached system. Supported examples include:

```text
Jumping from Mamet to Hoshoun
A Conduit Field activated by Clento Loso jumps you to Mai.
Activating a Conduit Field jumps you to Camal, bringing along 22 passengers.
```

The destination applies from that timestamp until the next movement or eligible location hint in the session. A jump's origin also establishes the earlier part of the session; before a first conduit message, location remains unknown unless ESI provides it. Coverage follows the session's timestamped entries, with a 45-second allowance after the last entry, rather than continuing across offline gaps.

The notification below is a weaker hint:

```text
(notify) Local spatial phenomena may cause strange effects on your ship systems.
```

With no prior logged jump, or strictly more than 30 seconds after the latest jump, it assumes **Wormhole** from that timestamp. A jump within the preceding 30 seconds suppresses it, including jumps in overlapping exports. Available ESI location at the event timestamp overrides this assumption; a later explicit jump replaces it. History labels the hint **Wormhole (assumed)**.

Log locations work without ESI sign-in. They are saved with session bounds in the encrypted history, including for unconnected characters, and remain available after source files leave the loaded set. Duplicate/overlapping exports do not duplicate movement events; growing files extend sessions. Matching ESI system IDs merge with logged names into one filter choice.
