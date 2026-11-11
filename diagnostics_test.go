package main

import (
	"fmt"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

func diagnosticGraphSection(report, name string) string {
	_, section, found := strings.Cut(report, "  "+name+": visible=")
	if !found {
		return ""
	}
	lines := strings.Split(section, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			return strings.Join(lines[:index], "\n")
		}
	}
	return section
}

func TestDiagnosticsReportPipelineAndReadOnlyState(t *testing.T) {
	ui, start := newMiningVolumeTestUI(t)
	ui.lastLogRefresh = start
	selected := maps.Clone(ui.selected)
	chart := ui.mining.data
	entries := append([]miningEvent(nil), ui.snapshot.Logs[0].Mining...)
	report := ui.diagnosticsReport()
	for _, required := range []string{
		fmt.Sprintf("Parser version: %d; cache schema: %d", logParserVersion, logCacheSchemaVersion),
		"Last successful refresh: 2026-11-11T02:00:00Z",
		"Range: [2026-11-11T02:00:00Z, 2026-11-11T02:00:04Z)",
		"damage hits=1, mining=5", "Character selections", "Filter state", "Graph pipeline",
		"Parsed snapshot SHA-256:", `"mining.txt"`, "positive points:", "Plot range:",
	} {
		if !strings.Contains(report, required) {
			t.Errorf("report omitted %q", required)
		}
	}
	if !strings.Contains(diagnosticGraphSection(report, "Mining"), "Loaded: 5; in range: 5; after filters/metric: 5; after character selection: 5") {
		t.Fatalf("incorrect mining stages: %s", report)
	}
	if !maps.Equal(selected, ui.selected) || !reflect.DeepEqual(chart, ui.mining.data) || !reflect.DeepEqual(entries, ui.snapshot.Logs[0].Mining) {
		t.Fatal("report changed selections, plotted data or loaded records")
	}
	ui.miningFilters[filterOreName].set("Loparite", false)
	ui.saveMiningFilters()
	report = ui.diagnosticsReport()
	if !strings.Contains(diagnosticGraphSection(report, "Mining"), "Loaded: 5; in range: 5; after filters/metric: 1; after character selection: 1") || !strings.Contains(report, `"Loparite"=false`) {
		t.Fatal("ore-filter exclusions were not reflected in diagnostic counts")
	}
	ui.selected["Miner"] = false
	ui.renderSelection()
	if !strings.Contains(diagnosticGraphSection(ui.diagnosticsReport(), "Mining"), "after character selection: 0") {
		t.Fatal("diagnostics missed deselected character")
	}
	ui.applyGraphRange(start.Add(time.Hour), start.Add(2*time.Hour))
	if !strings.Contains(diagnosticGraphSection(ui.diagnosticsReport(), "Mining"), "Loaded: 5; in range: 0") {
		t.Fatal("diagnostics missed time-range exclusion")
	}
}

func TestDiagnosticsNamedOutgoingGroupDoesNotBlameMining(t *testing.T) {
	ui, _ := newMiningVolumeTestUI(t)
	if _, err := ui.saveCharacterGroup(characterGroup{Name: "Absent", Members: []string{"Other Pilot"}}); err != nil {
		t.Fatal(err)
	}
	ui.groupSelect.SetSelected("Group: Absent")
	report := ui.diagnosticsReport()
	if !strings.Contains(diagnosticGraphSection(report, "Outgoing damage"), "after named outgoing group: 0") || !strings.Contains(diagnosticGraphSection(report, "Mining"), "after character selection: 5") {
		t.Fatal("report did not isolate outgoing grouping from mining selection")
	}
	entry := ui.snapshot.SourceFiles["mining.txt"]
	entry.Size = -1
	ui.snapshot.SourceFiles["mining.txt"] = entry
	if !strings.Contains(ui.diagnosticsReport(), "live prefix; cache bypassed") {
		t.Fatal("live snapshot cache state is missing")
	}
}

func TestErrorLogDiagnosticsRefreshAndCopy(t *testing.T) {
	ui, _ := newMiningVolumeTestUI(t)
	window := errorLogTestWindow{ui.window, test.NewClipboard()}
	var store runtimeErrorLog
	captures := 0
	viewer := newErrorLogDialog(window, &store, func() string { captures++; return ui.diagnosticsReport() })
	viewer.Show()
	defer viewer.Hide()
	text, _, buttons := errorLogDialogControls(window)
	if !text.Selectable || text.Text != "No errors recorded during this session." || captures != 0 {
		t.Fatal("opening Error log must display only errors and not generate diagnostics")
	}
	ui.selected["Miner"] = false
	ui.renderSelection()
	test.Tap(buttons["Refresh"])
	if text.Text != "No errors recorded during this session." || captures != 0 {
		t.Fatal("Refresh must display only session errors")
	}
	_, _ = store.Write([]byte("Test runtime failure\n"))
	test.Tap(buttons["Copy all"])
	copied := window.Clipboard().Content()
	if copied != text.Text || copied != "Test runtime failure\n" || captures != 0 {
		t.Fatal("Copy all must copy only session errors")
	}
	test.Tap(buttons["Copy diagnostic report"])
	copied = window.Clipboard().Content()
	if captures != 1 || !strings.Contains(diagnosticGraphSection(copied, "Mining"), "after character selection: 0") || strings.Contains(copied, "Test runtime failure") || text.Text != "Test runtime failure\n" {
		t.Fatal("diagnostic copy must capture current state without changing the error pane")
	}
	captureWindowPreview(t, window, os.Getenv("EVELA_DIAGNOSTICS_CAPTURE"))
}
