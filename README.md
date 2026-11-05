# Evela

Evela is a Windows desktop app for exploring EVE Online game logs across multiple characters. Compare damage, logistics, E-War and mining, find engagements, and inspect the events behind each graph.

## Get started

1. Download `evela.exe` or `evela.zip` from the [latest release](https://github.com/Losobie/evela/releases/latest). If you download the ZIP, extract it first.
2. Open `evela.exe`. No installer or database setup is needed.
3. Evela looks for logs in your Windows Documents folder under `EVE\logs\gamelogs`. To use another folder or combine logs from multiple computers, open **Settings → Log loading** and add one folder per line.
4. Open **Engagements** to find a session, or choose a time range in **Graphs** to start exploring.

By default, Evela loads files dated within the last **30 days**. To view an older archive, choose **Load all log files** in **Settings → Log loading**. **All history** in the time controls covers only the files you have loaded.

**Auto refresh** checks for new and changed logs every ten seconds. Use **Refresh logs** for an immediate refresh and **Details…** to check loading warnings. Large archives take longer on the first import; later launches reuse a local cache.

## Find your way around

| Tab | What you can do |
| --- | --- |
| **Graphs** | Explore outgoing and incoming damage, logistics, incoming E-War and mining over time. |
| **Totals** | Compare damage, average DPS and hit counts for the selected range and filters. Click column headings to sort. |
| **Events** | Search timestamped log messages and select a row to see its full message, ship, system and extracted fields. |
| **Engagements** | Browse automatically detected periods of activity with system and activity summaries. |
| **ESI & ships** | Connect characters to record ship and location history, and inspect that history. |

All timestamps use **EVE time (UTC)**.

### Find an engagement

Engagements are separated by at least **ten minutes without activity**. Summaries highlight the main activity, systems, targets and your ships, for example:

- **R3-K7K - Shooting Drake Navy Issue with Kikimora**
- **Sieh - Mining Xenotime with Exhumers**

Zero-value repairs and capacitor transfers do not create or extend engagements. Damage and sustained mining take priority in summaries.

Select a row to read its full summary. **Double-click a row**, or highlight it with the arrow keys and press **Enter**, to open Graphs with the engagement selected, plus 10% extra time before and after it. Existing graph filters still apply. The engagement list covers all loaded logs independently of the graph range and filters, and loads only when you open the tab.

### Explore graphs

Outgoing and incoming damage are shown initially. Open **Options** to show logistics, incoming E-War or mining, or hide graphs you do not need.

Choose a preset such as **30 minutes** or **All history**, or enter a **Custom** range using the date/time fields. Relative presets end at the latest logged activity, so they also work with old logs. Custom range ends are exclusive; use the next day's date to include a whole day.

| Action | Result |
| --- | --- |
| Scroll over a plot | Zoom in or out around the pointer. |
| Left-click and drag across a plot | Select a range; release to zoom. The readout shows the selected duration. |
| Hold right-click and drag | Move the timeline left or right. |
| **Ctrl+Z / Ctrl+Y**, or mouse **Back / Forward** | Undo or redo zooming and panning. |
| Hover over a line or point | Mark it with a dot and show its reading first, even outside the top three. |
| Click a line or point | Emphasize its series and dim the others; keep that series first in the readout. Click it again or click empty plot space to clear. |
| **Left / Right** | Pan earlier or later by a quarter of the visible time range. |
| **+ / −** (or **=**) | Zoom in or out around the center of the visible range. |
| **Escape** | Clear selected series across the graphs. |
| Scroll over a graph heading | Scroll the graph panel vertically. |

Keyboard graph navigation works in the Graphs tab when no control has keyboard focus; click a plot to return focus to navigation. Typing, tables, dialogs and menus retain their normal keyboard behavior.

Time navigation updates all graphs and Totals together. Graph intervals adjust automatically. **Options** also controls 10- or 30-second averaging; select the active average again to show individual events instead. Optional target invulnerability indicators mark supported structure notifications on the outgoing damage graph.

### Filter and group results

Use **Add filter** above the graphs to filter across graphs, or inside a graph to filter that graph. Checkboxes let you add several filters before clicking away to close the menu.

**Only visible filters apply.** Remove one with **×** or uncheck it in Add filter to stop applying it. Adding it again restores its previous selections. Each filter supports search, **All**, and individual choices. Choices within one filter combine; different filters narrow the result together.

Available choices depend on the graph: characters, character groups, ship type/class and system, plus **Weapon Type**, **Weapon Strength**, **Target**, **Target Type**, attackers and NPCs, logistics types, E-War effects, or ore and mining message types. Use **Group By** on the right of each graph to choose what its colored series represent. Outgoing damage grouping also controls Totals; damage filters update the matching direction in Totals.

Create named fleets or squads through **Settings → Character groups**. Groups can then be used as filters or combined into graph series. Filters, grouping and display preferences are saved between launches.

### Inspect events

**Events** follows the selected time range and the global **Characters** and **Character Group** filters. Other graph filters do not restrict it. Use **Message search** for text matching or enable **Regex** for regular expressions. Click **Timestamp** to reverse the order, and select an event to inspect or copy its full details.

## Optional ship and location tracking

Log analysis works without signing in. To record your ships and locations, open **Settings → ESI configuration → Character tokens** and choose **Sign in with EVE Online** for each character.

In **ESI & ships**, use **Refresh ships & locations**, enable automatic refresh, or open **Ship history…** and **Location history…**. Ship filters and grouping use the ship recorded at each event's time. Tracking cannot recover ships used before it began; missing history appears as unknown. Logged jumps can identify systems without ESI.

See [ESI setup and connection help](docs/ESI.md) for custom applications, permissions and sign-in troubleshooting.

## Appearance and local data

**Settings → Theme** offers **System**, **Light** and **Dark**. The default follows Windows, and your choice is saved.

Under **Settings → Theme → Graph colors**, choose **Protanopia (red-blind)**, **Deuteranopia (green-blind)**, **Tritanopia (blue-blind)** or **Achromatopsia (monochrome)** to adjust graph colors. The choice applies to lines, event points, hover readouts, character swatches and structure markers, works with either light or dark mode, and is saved between launches. Choose **Default** to restore the original colors. Hover over a line to identify it by name in crowded graphs.

Evela checks GitHub for a newer release at startup and every six hours while open. When one is available, **New release available** appears on the right of the bottom information panel. Click it to open that release's page and download the update. Checks run in the background; unavailable connections leave the app usable.

Evela reads English game logs without modifying or uploading them. SQLite caching is built in. Optional ESI connections send authorization and ship/location requests to CCP; saved tokens are encrypted for your Windows user.

For missing data, check **Details…**, the log-loading age limit, the selected time range and visible filters. See [data and history reference](docs/DATA.md) for parsing coverage, unknown values and cache locations.

## Development and licensing

**About → GitHub** opens the project repository. **About → Author** shows Losobie's GitHub profile, the in-game character **Clento Loso**, and the note that Evela was vibe coded using Codex.

Build, test, catalog regeneration, cache maintenance and profiling instructions are in the [developer guide](docs/DEVELOPMENT.md).

Evela's original code and assets use the [MIT License](LICENSE). Dependencies and CCP-derived game catalogs retain their own terms; the catalogs are outside the MIT grant and subject to CCP's EVE-related, non-commercial/non-profit terms. See [third-party notices](THIRD_PARTY_NOTICES.txt) or **About → Licenses**. EVE Online and related trademarks belong to CCP hf.; Evela is not affiliated with or endorsed by CCP. Details are in the [license maintenance guide](licenses/README.md).
