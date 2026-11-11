package main

import (
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

func diagnosticTime(value time.Time) string {
	if value.IsZero() {
		return "not available"
	}
	return value.UTC().Format(time.RFC3339)
}

// Generated on the UI thread only when Copy diagnostic report is clicked.
// Inspect published snapshots; never reopen source logs or change selections.
func (ui *analyzerUI) diagnosticsReport() string {
	var report strings.Builder
	fmt.Fprintf(&report, "Generated: %s\nRuntime: %s %s/%s\n", diagnosticTime(time.Now()), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if build, ok := debug.ReadBuildInfo(); ok {
		fmt.Fprintf(&report, "Application: %s %s\n", build.Main.Path, build.Main.Version)
		for _, setting := range build.Settings {
			if setting.Key == "vcs.revision" || setting.Key == "vcs.time" || setting.Key == "vcs.modified" {
				fmt.Fprintf(&report, "%s: %s\n", setting.Key, setting.Value)
			}
		}
		for _, dependency := range build.Deps {
			if dependency.Path == "fyne.io/fyne/v2" {
				fmt.Fprintf(&report, "Fyne: %s\n", dependency.Version)
			}
		}
	}
	fmt.Fprintf(&report, "Parser version: %d; cache schema: %d\n", logParserVersion, logCacheSchemaVersion)
	fmt.Fprintf(&report, "Fyne Do migration: %t\n", fyne.CurrentApp().Metadata().Migrations["fyneDo"])
	if currentBuildRevision() == "" {
		report.WriteString("Build revision: unavailable (no VCS metadata)\n")
	}
	if path, err := defaultLogCachePath(); err == nil {
		fmt.Fprintf(&report, "Cache path: %q\n", path)
	}
	fmt.Fprintf(&report, "Driver: %T; window: %v; scale: %g; appearance: %s; graph colors: %s\n", fyne.CurrentApp().Driver(), ui.window.Canvas().Size(), ui.window.Canvas().Scale(), savedAppearance(fyne.CurrentApp()), savedColorVision(fyne.CurrentApp()))
	fmt.Fprintf(&report, "Last successful refresh: %s\nRefresh status: %s\n", diagnosticTime(ui.lastLogRefresh), ui.status.Text)
	fmt.Fprintf(&report, "Folders: %q\nLoading limit: %d days (0 = all); cutoff: %s\n", ui.path.Text, ui.logLookbackDays, diagnosticTime(logLoadingCutoff(time.Now(), ui.logLookbackDays)))
	fmt.Fprintf(&report, "Auto refresh: %t; force reprocess pending: %t\n", ui.auto.Checked, ui.forceReprocess.Load())
	fmt.Fprintf(&report, "Range: [%s, %s), preset: %q\nBucket: %s; averaging: %s\n", diagnosticTime(ui.view.Start), diagnosticTime(ui.view.End), ui.rangePreset, ui.view.Bucket, ui.graphAverageWindow)
	snapshot := ui.snapshot
	if snapshot == nil {
		snapshot = &logSnapshot{}
	}
	fmt.Fprintf(&report, "Files: %d; cached: %d; processed: %d; too old: %d; undated skipped: %d; duplicate exports: %d; missing Listener: %d\n", snapshot.Files, snapshot.CachedFiles, snapshot.ParsedFiles, snapshot.SkippedOld, snapshot.SkippedUndated, snapshot.Duplicates, snapshot.Unassigned)
	fmt.Fprintf(&report, "Activity bounds: %s to %s\nLoaded records: %s\n", diagnosticTime(snapshot.First), diagnosticTime(snapshot.Last), diagnosticRecordCounts(snapshot))
	raw := snapshotInRange(snapshot, ui.view.Start, ui.view.End, 0)
	analysis := ui.analysisSnapshot()
	if analysis == nil {
		analysis = snapshot
	}
	filtered := snapshotInRange(analysis, ui.view.Start, ui.view.End, 0)
	fmt.Fprintf(&report, "Records in range: %s\nFooter: %s\n\n", diagnosticRecordCounts(raw), ui.summary.Text)
	report.WriteString("Character selections\n")
	names := map[string]bool{}
	for _, name := range snapshot.Characters {
		names[name] = true
	}
	for name := range ui.selected {
		names[name] = true
	}
	outgoingSelection := ui.outgoingCharacterSelection()
	for _, name := range diagnosticKeys(names) {
		fmt.Fprintf(&report, "  %q: selected=%t; outgoing selected=%t\n", name, ui.selected[name], outgoingSelection[name])
	}
	for _, group := range ui.characterGroups {
		fmt.Fprintf(&report, "  Saved group %q (%s): %q\n", group.Name, group.ID, group.Members)
	}
	report.WriteString("\nFilter state (inactive filters do not apply)\n")
	scopes := []string{globalFilterScope}
	for _, graph := range graphVisibilityOptions {
		scopes = append(scopes, graph.id)
	}
	for _, scope := range scopes {
		fmt.Fprintf(&report, "  %s\n", scope)
		for _, kind := range availableFilters(scope) {
			selection := ui.storedScopeFilter(scope, kind)
			fmt.Fprintf(&report, "    %s: active=%t", kind, ui.visibleFilters[scope][kind])
			if selection != nil {
				fmt.Fprintf(&report, "; default=%t; overrides:", selection.Default)
				for _, key := range diagnosticKeys(selection.Values) {
					fmt.Fprintf(&report, " %q=%t", key, selection.Values[key])
				}
			}
			report.WriteByte('\n')
		}
	}
	report.WriteString("\nGraph pipeline (range excludes averaging lookback; plotted points may include its contributions)\n")
	charts := map[string]*damageChart{graphOutgoing: ui.outgoing, graphIncoming: ui.incoming, graphOutgoingLogistics: ui.outgoingLogistics, graphIncomingLogistics: ui.incomingLogistics, graphIncomingEWar: ui.ewar, graphMining: ui.mining}
	for _, graph := range graphVisibilityOptions {
		chart := charts[graph.id]
		loaded := ui.diagnosticGraphRecords(snapshot, graph.id, nil, false)
		inRange := ui.diagnosticGraphRecords(raw, graph.id, nil, false)
		afterFilters := ui.diagnosticGraphRecords(filtered, graph.id, nil, true)
		afterSelection := ui.diagnosticGraphRecords(filtered, graph.id, ui.selected, true)
		afterGrouping := afterSelection
		grouping := ui.eventGraphGroupings[graph.id]
		if graph.id == graphOutgoing {
			grouping = ui.groupSelect.Selected
			afterGrouping = ui.diagnosticGraphRecords(filtered, graph.id, outgoingSelection, true)
		} else if graph.id == graphIncoming {
			grouping = ui.incomingGrouping
		}
		chart.mu.RLock()
		data := chart.data
		series, points, positive, invalid, hits := len(data.Series), 0, 0, 0, 0
		for _, line := range data.Series {
			hits += len(line.Hits)
			for _, value := range line.Values {
				points++
				if math.IsNaN(value) || math.IsInf(value, 0) {
					invalid++
				} else if value > 0 {
					positive++
				}
			}
		}
		chart.mu.RUnlock()
		fmt.Fprintf(&report, "  %s: visible=%t; size=%v; grouping=%q; units=%q/%q\n", graph.label, graphVisible(graph.id), chart.Size(), grouping, data.HitUnit, data.RateUnit)
		fmt.Fprintf(&report, "    Plot range: [%s, %s); bucket: %s; average: %s\n", diagnosticTime(data.Start), diagnosticTime(data.End), data.Bucket, data.AverageWindow)
		if graph.id == graphMining {
			fmt.Fprintf(&report, "    Volume mode: %t\n", ui.miningVolume)
		}
		if graph.id == graphIncomingEWar {
			fmt.Fprintf(&report, "    Metric: %q\n", ui.ewarMetric.Selected)
		}
		fmt.Fprintf(&report, "    Loaded: %d; in range: %d; after filters/metric: %d; after character selection: %d", loaded, inRange, afterFilters, afterSelection)
		if graph.id == graphOutgoing {
			fmt.Fprintf(&report, "; after named outgoing group: %d", afterGrouping)
		}
		report.WriteByte('\n')
		fmt.Fprintf(&report, "    Plotted series: %d; points: %d; positive points: %d; non-finite points: %d; individual events: %d; maximum: %g\n", series, points, positive, invalid, hits, data.Maximum)
		if series == 0 && data.EmptyText != "" {
			fmt.Fprintf(&report, "    Empty-state text: %q\n", data.EmptyText)
		}
		switch {
		case !graphVisible(graph.id):
			report.WriteString("    Graph hidden in Options.\n")
		case loaded == 0:
			report.WriteString("    No parsed records of this kind loaded.\n")
		case inRange == 0:
			report.WriteString("    No records in the selected range.\n")
		case afterFilters == 0:
			report.WriteString("    Filters or metric exclude every record in range.\n")
		case afterSelection == 0:
			report.WriteString("    No selected characters have matching records.\n")
		case afterGrouping == 0:
			report.WriteString("    Named outgoing group excludes every matching character.\n")
		case series == 0:
			report.WriteString("    Matching records exist but grouping produced no series.\n")
		case invalid > 0:
			report.WriteString("    Plot contains invalid numeric values.\n")
		case chart.Size().Width <= 0 || chart.Size().Height <= 0:
			report.WriteString("    Graph has no drawable area.\n")
		case positive == 0:
			report.WriteString("    Plot has no positive values; check misses or zero amounts.\n")
		}
	}
	report.WriteString("\nSource files (range matches first; up to 200 files)\n")
	paths := diagnosticKeys(snapshot.SourceFiles)
	sort.SliceStable(paths, func(i, j int) bool {
		return diagnosticFileInRange(snapshot.SourceFiles[paths[i]].Log, ui.view.Start, ui.view.End) && !diagnosticFileInRange(snapshot.SourceFiles[paths[j]].Log, ui.view.Start, ui.view.End)
	})
	for _, path := range paths[:min(len(paths), 200)] {
		entry := snapshot.SourceFiles[path]
		if entry.Log == nil {
			continue
		}
		state := "complete snapshot"
		size := fmt.Sprint(entry.Size)
		if entry.Size < 0 {
			state = "live prefix; cache bypassed"
			size = "not cached"
		}
		fmt.Fprintf(&report, "  %q\n    Character: %q; bytes: %s; modified: %s; %s\n    Parsed snapshot SHA-256: %x; in range: %t\n    %s\n", path, entry.Log.Character, size, diagnosticTime(entry.Modified), state, entry.Log.Hash, diagnosticFileInRange(entry.Log, ui.view.Start, ui.view.End), diagnosticRecordCounts(&logSnapshot{Logs: []*parsedLog{entry.Log}}))
	}
	if len(paths) > 200 {
		fmt.Fprintf(&report, "  %d more source files omitted.\n", len(paths)-200)
	}
	for _, warning := range snapshot.Warnings {
		fmt.Fprintf(&report, "Loading warning: %s\n", warning)
	}
	return report.String()
}

func diagnosticKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func diagnosticRecordCounts(snapshot *logSnapshot) string {
	events, hits, mining, logistics, ewar := 0, 0, 0, 0, 0
	for _, log := range snapshot.Logs {
		events += len(log.Events)
		hits += len(log.Hits)
		mining += len(log.Mining)
		logistics += len(log.Logistics)
		ewar += len(log.EWar)
	}
	return fmt.Sprintf("events=%d, damage hits=%d, mining=%d, logistics=%d, E-War=%d", events, hits, mining, logistics, ewar)
}

func diagnosticFileInRange(log *parsedLog, start, end time.Time) bool {
	if log == nil {
		return false
	}
	return len(recordsInRange(log.Events, start.Unix(), end.Unix(), func(event logEvent) int64 { return event.Second })) > 0
}

func (ui *analyzerUI) diagnosticGraphRecords(snapshot *logSnapshot, scope string, selected map[string]bool, localFilters bool) int {
	count := 0
	for _, log := range snapshot.Logs {
		if selected != nil && !selected[log.Character] {
			continue
		}
		switch scope {
		case graphOutgoing, graphIncoming:
			incoming := scope == graphIncoming
			if len(log.Hits) > 0 {
				for _, hit := range log.Hits {
					if hit.Incoming == incoming {
						count++
					}
				}
			} else {
				for _, point := range log.Points {
					if incoming && (point.Incoming > 0 || point.IncomingHits > 0) || !incoming && (point.Outgoing > 0 || point.OutgoingHits > 0) {
						count++
					}
				}
			}
		case graphMining:
			for _, event := range log.Mining {
				if localFilters && !ui.miningEventSelected(event) {
					continue
				}
				if localFilters && ui.miningVolume {
					if _, known := miningEventVolume(event); !known {
						continue
					}
				}
				count++
			}
		case graphIncomingLogistics, graphOutgoingLogistics:
			for _, event := range log.Logistics {
				if event.Incoming == (scope == graphIncomingLogistics) && (!localFilters || ui.scopeFilter(scope, filterLogisticsType).selected(event.Kind)) {
					count++
				}
			}
		case graphIncomingEWar:
			for _, event := range log.EWar {
				if !localFilters || ui.ewarMetric.Selected != ewarCapacitor || event.Effect == "Energy neutralizer" || event.Effect == "Nosferatu" {
					count++
				}
			}
		}
	}
	return count
}
