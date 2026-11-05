package main

import (
	"encoding/json"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func newRangePreferenceTestUI(t *testing.T, application fyne.App, snapshot *logSnapshot) *analyzerUI {
	t.Helper()
	window := application.NewWindow("Saved time range")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	ui.snapshot = snapshot
	ui.updateRange() // The initial log import updates relative presets.
	return ui
}

func TestGraphRangePresetPreference(t *testing.T) {
	application := test.NewTempApp(t)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{First: start, Last: start.Add(2 * time.Hour)}
	ui := newRangePreferenceTestUI(t, application, snapshot)
	if ui.rangePreset != "Last 30 minutes" {
		t.Fatal("a new user did not get the default time range")
	}
	for _, preset := range quickRangeOptions {
		if preset == selectDayOption {
			continue
		}
		t.Run(preset, func(t *testing.T) {
			ui.selectQuickRange(preset)
			later := &logSnapshot{First: start.Add(-time.Hour), Last: snapshot.Last.Add(48 * time.Hour)}
			restored := newRangePreferenceTestUI(t, application, later)
			wantStart, wantEnd := presetRange(preset, later)
			if restored.rangePreset != preset || !restored.view.Start.Equal(wantStart) || !restored.view.End.Equal(wantEnd) {
				t.Fatalf("preset did not restore against the newly loaded logs: %+v", graphHistoryState(restored))
			}
			var saved graphRangePreferenceState
			if err := json.Unmarshal([]byte(application.Preferences().String(graphRangePreference)), &saved); err != nil || saved.Start != "" || saved.End != "" {
				t.Fatal("a relative preset saved fixed date bounds")
			}
		})
	}
}

func TestGraphRangeCustomPreferenceAndNavigation(t *testing.T) {
	application := test.NewTempApp(t)
	snapshot := &logSnapshot{}
	ui := newRangePreferenceTestUI(t, application, snapshot)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.applyGraphRange(start, start.Add(time.Hour))
	ui.updateRange()
	assertRestored := func() {
		t.Helper()
		want := graphHistoryState(ui)
		restored := newRangePreferenceTestUI(t, application, snapshot)
		if got := graphHistoryState(restored); got != want {
			t.Fatalf("restored range = %+v, want %+v", got, want)
		}
		if len(restored.graphUndo)+len(restored.graphRedo) != 0 {
			t.Fatal("restoring the preference created undo/redo history")
		}
	}
	assertRestored()
	ui.applyGraphRange(ui.view.Start.Add(5*time.Minute), ui.view.End.Add(-5*time.Minute))
	assertRestored()
	ui.panGraphRange(true)
	assertRestored()
	ui.applyGraphWheelRange(ui.view.Start.Add(time.Minute), ui.view.End.Add(-time.Minute))
	assertRestored()
	ui.undoGraphRange()
	assertRestored()
	ui.redoGraphRange()
	assertRestored()
	// Invalid programmatic bounds must not replace the last applied range.
	saved := application.Preferences().String(graphRangePreference)
	ui.applyGraphRange(ui.view.End, ui.view.Start)
	if application.Preferences().String(graphRangePreference) != saved {
		t.Fatal("invalid bounds overwrote the saved range")
	}
	assertRestored()
}

func TestGraphRangeInvalidPreferenceFallsBack(t *testing.T) {
	for _, data := range []string{
		"not json",
		`{"preset":"removed preset"}`,
		`{"preset":"Last 30 seconds"}`,
		`{"preset":"Last 1 minute"}`,
		`{"preset":"Last 5 minutes"}`,
		`{"preset":"Latest combat day"}`,
		`{"preset":"Custom"}`,
		`{"preset":"Custom","start":"bad date","end":"2026-10-18"}`,
		`{"preset":"Custom UTC range"}`,
		`{"preset":"Custom UTC range","start":"bad date","end":"2026-10-18"}`,
		`{"preset":"Custom UTC range","start":"2026-10-18","end":"2026-10-17"}`,
		`{"preset":"Custom UTC range","start":"2026-10-18","end":"2026-10-18"}`,
	} {
		t.Run(data, func(t *testing.T) {
			application := test.NewTempApp(t)
			application.Preferences().SetString(graphRangePreference, data)
			ui := newRangePreferenceTestUI(t, application, &logSnapshot{})
			if ui.rangePreset != "Last 30 minutes" || !ui.view.End.After(ui.view.Start) {
				t.Fatalf("invalid preference did not fall back to a valid default: %+v", graphHistoryState(ui))
			}
		})
	}
}

func TestGraphRangeLegacyCustomPreference(t *testing.T) {
	application := test.NewTempApp(t)
	application.Preferences().SetString(graphRangePreference, `{"preset":"Custom UTC range","start":"2026-10-17 02:00:00","end":"2026-10-17 03:00:00"}`)
	ui := newRangePreferenceTestUI(t, application, &logSnapshot{})
	wantStart := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	if ui.rangePreset != "Custom" || !ui.view.Start.Equal(wantStart) || !ui.view.End.Equal(wantStart.Add(time.Hour)) {
		t.Fatalf("legacy custom range lost its saved bounds: %+v", graphHistoryState(ui))
	}
	var saved graphRangePreferenceState
	if err := json.Unmarshal([]byte(application.Preferences().String(graphRangePreference)), &saved); err != nil || saved.Preset != "Custom" {
		t.Fatal("legacy custom preference was not saved with the new name")
	}
}
