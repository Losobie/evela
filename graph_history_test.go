package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

// The Fyne test canvas hides its shortcut dispatcher behind an interface.
// Capture the actual registrations while delegating layout/focus to that canvas.
type graphShortcutTestCanvas struct {
	fyne.Canvas
	fyne.ShortcutHandler
}

func (canvas *graphShortcutTestCanvas) AddShortcut(shortcut fyne.Shortcut, handler func(fyne.Shortcut)) {
	canvas.ShortcutHandler.AddShortcut(shortcut, handler)
	canvas.Canvas.AddShortcut(shortcut, handler)
}

func (canvas *graphShortcutTestCanvas) RemoveShortcut(shortcut fyne.Shortcut) {
	canvas.ShortcutHandler.RemoveShortcut(shortcut)
	canvas.Canvas.RemoveShortcut(shortcut)
}

type graphShortcutTestWindow struct {
	fyne.Window
	canvas *graphShortcutTestCanvas
}

func (window *graphShortcutTestWindow) Canvas() fyne.Canvas { return window.canvas }

func newGraphHistoryTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	application := test.NewTempApp(t)
	window := application.NewWindow("Graph undo/redo")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(&graphShortcutTestWindow{Window: window, canvas: &graphShortcutTestCanvas{Canvas: window.Canvas()}})
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{First: start, Last: start.Add(3599 * time.Second), Characters: []string{"Pilot A", "Pilot B"}, Logs: []*parsedLog{
		{Character: "Pilot A", Points: []damagePoint{
			{Second: start.Unix(), Outgoing: 1000, Incoming: 100},
			{Second: start.Add(30 * time.Minute).Unix(), Outgoing: 30, Incoming: 3},
			{Second: start.Add(45 * time.Minute).Unix(), Outgoing: 60, Incoming: 6},
			{Second: start.Add(59 * time.Minute).Unix(), Outgoing: 90, Incoming: 9},
		}},
		{Character: "Pilot B", Points: []damagePoint{{Second: start.Add(30 * time.Minute).Unix(), Outgoing: 300, Incoming: 30}}},
	}}
	ui.selected["Pilot A"], ui.selected["Pilot B"] = true, false
	ui.rangeSelect.SetSelected("Last hour")
	applyTestShipRecords(ui, []shipRecord{{CharacterName: "Pilot A", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()}, {CharacterName: "Pilot B", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()}})
	ui.groupSelect.SetSelected("Ship history")
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	return ui, start
}

func graphHistoryState(ui *analyzerUI) graphRangeState {
	return graphRangeState{Start: ui.view.Start, End: ui.view.End, Preset: ui.rangeSelect.Selected}
}

func assertGraphHistoryState(t *testing.T, ui *analyzerUI, want graphRangeState, totalOut float64) {
	t.Helper()
	if !ui.view.Start.Equal(want.Start) || !ui.view.End.Equal(want.End) || ui.rangeSelect.Selected != want.Preset {
		t.Fatalf("wrong range: got %+v, want %+v", graphHistoryState(ui), want)
	}
	if ui.start.Text != want.Start.UTC().Format("2006-01-02 15:04:05") || ui.end.Text != want.End.UTC().Format("2006-01-02 15:04:05") {
		t.Fatal("UTC fields did not follow undo/redo")
	}
	for _, chart := range []*damageChart{ui.incoming, ui.outgoing} {
		if !chart.data.Start.Equal(want.Start) || !chart.data.End.Equal(want.End) {
			t.Fatal("graph time axis did not follow undo/redo")
		}
	}
	if ui.view.Series[0].TotalOutgoing != totalOut {
		t.Fatalf("wrong filtered damage: got %v, want %v", ui.view.Series[0].TotalOutgoing, totalOut)
	}
	if !ui.shipGrouping() || !ui.selected["Pilot A"] || ui.selected["Pilot B"] {
		t.Fatal("range history changed grouping or character selection")
	}
}

func sendGraphShortcut(ui *analyzerUI, shortcut fyne.Shortcut) {
	ui.window.Canvas().(fyne.Shortcutable).TypedShortcut(shortcut)
}

func dragGraphFraction(chart *damageChart, fromFraction, toFraction float32) {
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	from, to := fyne.NewPos(chartPlotLeft+width*fromFraction, 80), fyne.NewPos(chartPlotLeft+width*toFraction, 80)
	chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(from, to))
	chart.DragEnd()
}

func TestGraphRangeUndoRedoShortcuts(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	dragGraphFraction(ui.outgoing, .25, .9)
	first := graphHistoryState(ui)
	assertGraphHistoryState(t, ui, first, 90)
	if ui.tableSeries[0].TotalOutgoing != 390 {
		t.Fatal("ship totals did not follow first selection")
	}
	dragGraphFraction(ui.incoming, .1, .6)
	second := graphHistoryState(ui)
	assertGraphHistoryState(t, ui, second, 30)
	if ui.tableSeries[0].TotalOutgoing != 330 {
		t.Fatal("ship totals did not follow second selection")
	}
	ui.updateRange()
	ui.renderSelection()
	if len(ui.graphUndo) != 2 || len(ui.graphRedo) != 0 {
		t.Fatal("refresh changed drag history")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, first, 90)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, first, 90)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, second, 30)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, second, 30)
	if len(ui.graphUndo) != 2 || len(ui.graphRedo) != 0 {
		t.Fatal("undo/redo recorded extra history")
	}
}

func TestGraphRangeRedoBranchesAndManualChanges(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	ui.applyGraphRange(start.Add(15*time.Minute), start.Add(54*time.Minute))
	first := graphHistoryState(ui)
	ui.applyGraphRange(start.Add(25*time.Minute), start.Add(40*time.Minute))
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	// Selecting the same interval is not a new edit and preserves redo.
	ui.applyGraphRange(ui.view.Start, ui.view.End)
	if len(ui.graphRedo) != 1 || len(ui.graphUndo) != 1 {
		t.Fatal("no-op selection changed history")
	}
	ui.applyGraphRange(start.Add(35*time.Minute), start.Add(50*time.Minute))
	branch := graphHistoryState(ui)
	if len(ui.graphRedo) != 0 {
		t.Fatal("new selection retained the abandoned redo branch")
	}
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, branch, 60)
	ui.bucketSelect.SetSelected("5 seconds")
	ui.updateRange()
	if len(ui.graphUndo) != 2 {
		t.Fatal("interval change or refresh erased range history")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, first, 90)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	ui.rangeSelect.SetSelected("All history")
	if len(ui.graphUndo)+len(ui.graphRedo) != 0 {
		t.Fatal("manual preset change retained stale drag history")
	}
	ui.applyGraphRange(start.Add(15*time.Minute), start.Add(54*time.Minute))
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	ui.start.SetText(start.Add(time.Minute).Format("2006-01-02 15:04:05"))
	if len(ui.graphUndo)+len(ui.graphRedo) != 0 {
		t.Fatal("manual date edit retained stale drag history")
	}
}

func TestGraphRangeShortcutsRespectTextAndDialogs(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	ui.applyGraphRange(start.Add(15*time.Minute), start.Add(54*time.Minute))
	zoom := graphHistoryState(ui)
	ui.window.Canvas().Focus(ui.characterSearch)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, zoom, 90)
	ui.window.Canvas().Unfocus()
	ui.openESISettings()
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, zoom, 90)
	ui.esiDialog.Hide()
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, zoom, 90)
}

func TestGraphRangeUndoCancelsPendingDrag(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	ui.applyGraphRange(start.Add(15*time.Minute), start.Add(54*time.Minute))
	zoom := graphHistoryState(ui)
	chart := ui.incoming
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	from, to := fyne.NewPos(chartPlotLeft+width*.2, 80), fyne.NewPos(chartPlotLeft+width*.8, 80)
	chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(from, to))
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	chart.DragEnd()
	assertGraphHistoryState(t, ui, initial, 1180)
	if len(ui.graphUndo) != 0 || len(ui.graphRedo) != 1 {
		t.Fatal("pending drag reapplied itself after undo")
	}
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, zoom, 90)
}
