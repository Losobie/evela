# EVE game-log parser

A standalone Go module maintained inside Evela: `github.com/Losobie/evela/parser`.
It has no Fyne, SQLite, ESI, operating-system storage or CGO dependency. Its only
external dependency is `golang.org/x/text` for UTF-16 decoding.

```go
log, err := parser.Parse(reader)
if err != nil {
    return err
}
for _, hit := range log.Hits {
    fmt.Println(log.Character, hit.Amount, hit.WeaponType, hit.TargetType)
}
```

`Parse` reads one English EVE log from an `io.Reader`. Results include raw events,
damage hits and per-second totals, incoming sources, logistics, E-War, mining,
movement hints and structure notifications. Optional metadata stays empty when
the log does not supply it. `ActivityBounds` returns the first/last supported
activity, excluding travel/chat; `Hash` identifies the original input bytes.

The package documentation explains timestamps, source indices and ownership.
Exact catalog lookups are available through `HullClass`, `NPCGroup`, `LookupNPC`
and `OreType`. `OreVolume` returns cubic metres per unit and whether the volume
is known. Catalogs load lazily and work offline. They are not writable API
state. Cross-file merging, historical location inference, caching and graph
queries remain in the desktop application.

Run tests from this directory with `go test ./...`, or from the repository root
with `go -C parser test ./...`. `CGO_ENABLED=0` works. The root module uses a local
`replace` directive, so changes are available to Evela without publishing a
module version. A future separate repository can copy this directory, choose
its module path, and update Evela's import/require/replace settings.

Original code uses the [MIT license](LICENSE). Bundled CCP-derived catalogs are
outside that grant; preserve [NOTICE](NOTICE) and comply with CCP's terms when
redistributing them. Consumers distributing compiled programs must also retain
their Go/runtime and dependency notices. Catalog generation and provenance are
documented in Evela's [developer guide](../docs/DEVELOPMENT.md).
