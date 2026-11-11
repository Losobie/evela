# UI threading audit

Audited on 2026-11-11 against Fyne v2.8.0 and its
[migration guidance](https://docs.fyne.io/explore/migrations/).

Evela opts into Fyne's `fyneDo` migration. `app_metadata.go` declares it before
driver construction, so the standard standalone `go build` works without an
external metadata file or extra build tags. `FyneApp.toml` declares the same
migration for the Fyne packaging tool. Do not filter the compatibility warning
out of the error log or use `no_metadata` to hide it.

## Ownership boundaries

| Path | Background work | UI handoff |
| --- | --- | --- |
| `log_worker.go`, `logs.go`, `multi_log_index.go` | Folder scans, parsing, SQLite cache; cancellation and force flag are channels/atomics | Widget reads, progress, snapshot publication and graph rebuilding use `fyne.Do` / `fyne.DoAndWait` |
| `esi_ui.go`, `esi_login.go`, `esi_auth.go` | Worker-owned service configuration, HTTP/SSO, history persistence and callback server | Settings reads, connection views, browser launch and login/refresh controls use `fyne.DoAndWait`; timer updates use `fyne.Do` |
| `engagements_ui.go` | Captured immutable log snapshot and cloned history; generation guards discard obsolete results | Captured `dispatch` is `fyne.Do`; result widgets update only inside the dispatched closure |
| `releases.go` | HTTP requests and timer | Release link updates use `fyne.Do` |
| `graph_mouse_windows.go` | Native Win32 callback | Non-blocking `fyne.Do` defers navigation until after the native event; never wait on this callback |
| `main.go`, `window_geometry_windows.go` | No worker widget access | Initial construction precedes the run loop; Fyne event/lifecycle handlers own UI state; native handles are accessed through `RunNative` |
| `profiling.go`, `error_log.go` | HTTP profiler and diagnostic writes | Profiler has no UI access; error buffer is mutex protected; report/clipboard actions run in UI callbacks |
| Chart renderers and input handlers | No application-created UI worker | Toolkit callbacks; chart plot data has its own mutex; parser/storage workers never operate on widgets |

Workers communicate through channels, immutable snapshots, cloned history or
explicit atomics. Do not read widget fields from workers, including checkbox
values. Never call `fyne.DoAndWait` inside UI callbacks; it would wait on the
same event loop. A native callback may enqueue non-blocking `fyne.Do` work.

## Verification

Run the normal root and parser suites, and also:

```powershell
go test -tags migrated_fynedo ./...
go test -tags ci -run '^TestFyneDoStartup$' .
```

The first exercises Fyne internals with legacy threading compatibility removed.
The second uses production startup metadata and Fyne's real application run
path with a software driver, verifying the migration warning is absent. It
must run separately because Fyne caches migration state once per process.
These checks do not replace testing native desktop interaction.
