# Code map

Evela has two Go modules. The root builds the desktop application; `parser/` is
an independent library. There is no workspace setup or generation step needed
to build the application: the root `go.mod` resolves the parser locally.

| Area | Start here | Responsibility |
| --- | --- | --- |
| Startup and UI | `main.go`, `ui.go` | App lifecycle, widget ownership and layout |
| Parsing | [`parser/`](../parser/README.md) | Log decoding, extracted records and embedded catalogs |
| Parser boundary | `log_parser.go` | Local aliases, without a second model or conversions |
| Log loading/cache | `log_worker.go`, `logs.go`, `multi_log_index.go`, `log_cache.go` | Background folder scans, immutable snapshots, deduplication and SQLite cache |
| Live log reads | `live_log.go` | Fixed-size reads through complete UTF-8/UTF-16 lines; growing prefixes bypass caching |
| Analysis | `analysis.go`, `graph_view.go`, `graph_window.go`, `dps_average.go` | Ranges, filtered snapshots, graph/totals updates |
| Event plots | `event_plot.go`, `logistics.go`, `mining.go`, `ewar.go` | Shared bucketing/averaging, with graph-specific filtering and units |
| Mining totals | `mining_totals.go` | Ore rows, critical bonus breakdowns and volume conversion using parser catalogs |
| Chart widget | `chart.go`, `chart_data.go`, `chart_render.go` | Widget layout, plot records and drawing |
| Chart input | `chart_inspection.go`, `chart_drag.go`, `chart_line_hover.go`, `chart_series_selection.go` | Hover, range selection and series emphasis |
| Timeline navigation | `quick_range.go`, `graph_history.go`, `graph_pan.go`, `graph_wheel.go` | Presets, day selection, keyboard/mouse changes and undo/redo |
| Filters/grouping | `scoped_filters.go`, `scoped_filter_snapshot.go`, `graph_filter_catalog.go`, `event_graph_grouping.go` | Available choices, active predicates and series grouping |
| Events/engagements | `events.go`, `event_details.go`, `engagements.go`, `engagement_summary.go`, `engagements_ui.go` | Searchable records and activity discovery |
| ESI/history | `esi*.go`, `ship_history.go`, `location_history.go`, `log_location.go` | Optional SSO, observations and time-based attribution |
| Appearance | `appearance.go`, `color_vision.go` | Theme and graph accessibility colors |
| Runtime diagnostics | `error_log.go`, `diagnostics.go` | Session errors and copyable reports of loading, filters, selections and graph pipelines |
| Maintenance | `tools/`, `internal/sde/`, `licenses/` | Catalog generation, shared SDE reading and distribution notices |

## Boundaries worth preserving

- The parser accepts readers and returns records. It does not discover files,
  merge exports, call ESI, store preferences or calculate graph rates.
- Loaded logs are shared as immutable snapshots. Filters build views rather
  than modifying cached records. The UI owns its widgets; workers publish
  results through Fyne's UI dispatch.
- Graph-specific code chooses events, grouping names and units. The event plot
  builder owns time clipping, rolling-window contributions, hit sorting and
  partial buckets. HP, GJ and mining message kinds remain separate.
- History and engagement logic stay outside the parser: their results depend
  on multiple logs, observed ships/locations and user choices.
- Theme, hover and emphasis changes should refresh renderers without reparsing
  logs or rebuilding queries.

## Making changes

Run `go test ./...` and `go -C parser test ./...`; the first command does not
descend into the second module. Existing app integration tests exercise the
parser boundary, while parser tests also run without native desktop libraries.
Tests use mock servers rather than live credentials.

When changing parser records or interpretation, increment `logParserVersion` in
`log_cache.go`, even if a source file's contents have not changed. SQL schema
changes additionally need a migration and schema-version update. Catalog
generators write into `parser/`; preserve their CCP notices.

Prefer a shared helper for repeated domain behavior with matching semantics.
Keep different filtering, units and grouping decisions visible at their call
sites. Splitting files by responsibility is useful; introducing a generic
framework or interfaces for a single implementation usually adds reading work.

See [DEVELOPMENT.md](DEVELOPMENT.md) for build, release, profiling and catalog
commands and [AGENTS.md](../AGENTS.md) for repository work instructions.
