# Developing Evela

[User guide](../README.md) · [Data and history](DATA.md) · [ESI setup](ESI.md) · [License maintenance](../licenses/README.md)

## Run, build and test

Evela uses Go, Fyne and SQLite. Development requires Go 1.23 or later and [Fyne's desktop build prerequisites](https://docs.fyne.io/started/), including a C compiler. On Windows, use MinGW GCC. Python 3 is needed for distribution-notice maintenance.

Run from the repository root:

```powershell
go mod download
go run .
```

Verify and build the Windows application:

```powershell
python tools/licenses/generate.py --check
go test ./...
go build -ldflags="-H=windowsgui" -o evela.exe .
Compress-Archive -LiteralPath evela.exe -DestinationPath evela.zip -Force
```

Use `evela.exe` for Windows builds and replace the existing file. After each successful build, replace `evela.zip` with an archive containing only that executable. Build outputs are ignored by Git. See [AGENTS.md](../AGENTS.md) for repository contribution instructions.

The application icon comes from `icon.svg`. After editing it, run `go generate .` to regenerate `icon.ico` and `icon_windows_amd64.syso`, then rebuild. Resource generation requires MinGW `windres`.

### Tests

The test suite uses local mock SSO/ESI servers and fake credentials. It covers login, token validation and renewal, encrypted storage, ship changes, rate limits, parsing, filters and graph interactions. Windows DPAPI tests need access to the user's encryption profile; a restricted sandbox may require running the suite outside that sandbox.

An optional read-only integration test checks a real log archive:

```powershell
$env:EVELA_TEST_LOGS = Join-Path ([Environment]::GetFolderPath('MyDocuments')) 'EVE\logs\gamelogs'
go test -run TestLocalLogs -v
```

## Releases and notices

The [release workflow](../.github/workflows/release.yml) runs on pushes to `main`, builds Windows x64, and publishes only after tests pass. Each release uses the tag `build-<full commit SHA>` and attaches `evela.exe`, `evela.zip`, `LICENSE` and `THIRD_PARTY_NOTICES.txt`. The ZIP contains only the executable; notices are also embedded and available through **About → Licenses**.

New pushes cancel older runs. A run checks that its commit is still the tip of `main` before publishing, and rerunning that commit updates its existing release. The workflow uses GitHub's built-in token, with no additional secrets. Go module/build caches and the MSYS2 action cache reduce repeat setup work. Cold Fyne/CGO builds can take several minutes; separate download, compile and test steps provide progress and time limits.

Update checks read Go's embedded `vcs.revision` and compare it with the latest published `build-<SHA>` release through GitHub's public API. A link appears only when the release commit is ahead of the running build; older, identical and diverged commits are excluded. CI verifies that the executable's revision matches the release commit. Build from a Git checkout with VCS stamping enabled (the default); builds without revision metadata skip update checks. Checks run at startup and every six hours, with a ten-second timeout and no authentication or log uploads.

After dependency or compiler changes, review upstream terms and regenerate notices:

```powershell
go mod download
python tools/licenses/generate.py
python tools/licenses/generate.py --check
```

For a different Windows compiler distribution, supply its actual license directory:

```powershell
python tools/licenses/generate.py --native-root C:\msys64\mingw64\share\licenses
```

CI checks the committed notices, then collects native notices from the compiler used for that release before building. The collector does not replace review of new dependencies or linked libraries. See the [license maintenance guide](../licenses/README.md) for scope, exceptions and redistribution requirements, including CCP data terms.

## Embedded game catalogs

The compact catalogs come from CCP's [official Static Data Export](https://developers.eveonline.com/docs/services/static-data/), JSONL build **3607705**, released **2026-10-12**. They work offline and load lazily.

| File | Contents | SDE selection |
| --- | --- | --- |
| `npc_data.json.gz` | 7,418 NPC types and 415 groups, with all supplied name translations | Entity category, `categoryID = 11`; includes unpublished types |
| `ore_data.json` | 502 English ore names, including ice and compressed variants | Asteroid category, `categoryID = 25` |
| `hull_data.json` | 834 English ship, starbase and structure names with classes | Categories 6, 23 and 65 |

Types join their `groupID` to `groups._key`. Ore group labels replace “Moon Asteroids” with “Moon Ore” for display. Only compact names, type/group IDs and group names are embedded; the full archive stays out of version control.

Download the [latest JSONL SDE](https://developers.eveonline.com/static-data/eve-online-static-data-latest-jsonl.zip) into `.cache`, then substitute its filename below:

```powershell
go run ./tools/npcdata -sde .cache/eve-sde-<build>-jsonl.zip
go run ./tools/oredata -sde .cache/eve-sde-<build>-jsonl.zip
go run ./tools/hulldata -sde .cache/eve-sde-<build>-jsonl.zip
```

The generators read build/release metadata from the archive. Update this page's snapshot details and counts, increment the parser version when the changed catalog affects parsed results, then test, build and recreate the ZIP as above. CCP-derived catalogs retain CCP's terms and are excluded from Evela's MIT grant.

## Parsed-log cache maintenance

`logParserVersion` in `log_cache.go` is independent of the SQLite schema version. Increment it when changing extracted fields, parsing semantics, source classification, relevant embedded catalogs or the serialized `parsedLog` representation. Cache keys include this version, so eligible unchanged source files are reprocessed on import. Files excluded by loading settings are reprocessed when subsequently included.

SQL table changes additionally require a migration and a `logCacheSchemaVersion` update. Parser-version changes alone do not require a migration.

SQLite stores compressed complete parsed records keyed by source-content hash and parser version. A separate indexed manifest records file path, size and modification time. Identical exports share a payload. Payload and manifest updates commit together; superseded payloads from growing logs are removed when no manifest references them.

Graphs use the in-memory query engine. SQLite avoids repeat parsing across launches; it is not a SQL graph-query backend. See [loading and storage behavior](DATA.md#loading-and-storage) before changing invalidation or folder handling.

## Interaction performance and profiling

Graph accessibility colors are applied at rendering time in `color_vision.go`; the stable series identities and query data stay unchanged. Protanopia, deuteranopia and tritanopia modes use the full-severity linear-sRGB matrices from [Machado, Oliveira and Fernandes (2009)](https://www.inf.ufrgs.br/~oliveira/pubs_files/CVD_Simulation/CVD_Simulation.html) to estimate lost color information, then redistribute that difference into the other channels. This is a correction, rather than displaying the simulation itself. Monochrome mode maps hue to gray levels. Contrast is adjusted for the active light/dark plot background. These are original implementations using numerical model coefficients, with no new runtime dependency. Tests cover representative confusable pairs, contrast, menu persistence and matching line/hover/swatch colors. Simulations help evaluate the choices; user feedback remains important for individual vision differences.

Events refresh only while their tab is visible. Engagements are built on demand in the background, cached across navigation and invalidated by log or history changes; leaving the tab cancels unfinished work. Graph updates share ship-history indexes and timestamp metadata, reuse filter-option keys and retain unchanged record slices. New snapshots and metadata refreshes invalidate these caches.

To profile a local interactive session, enable the opt-in loopback pprof server before launching Evela. Normal launches start no diagnostic server.

```powershell
New-Item -ItemType Directory -Path .cache -Force | Out-Null
$env:EVELA_PROFILE_ADDR = '127.0.0.1:6060'
Start-Process -FilePath .\evela.exe
Invoke-WebRequest 'http://127.0.0.1:6060/debug/pprof/profile?seconds=60' -OutFile .cache/live.cpu
Invoke-WebRequest 'http://127.0.0.1:6060/debug/pprof/trace?seconds=10' -OutFile .cache/live.trace
go tool pprof -top .\evela.exe .cache/live.cpu
Remove-Item Env:EVELA_PROFILE_ADDR
```

Use the application during capture and keep it open until recording finishes. CPU profiles and traces are captured on demand; blocking and mutex samples are collected while profiling is enabled. Files remain in the requested local directory. Closing Evela stops its server; removing the environment variable makes subsequent launches normal again.
