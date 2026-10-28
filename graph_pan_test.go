package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func graphPanButtons(root fyne.CanvasObject) []*graphPanButton {
	var buttons []*graphPanButton
	switch object := root.(type) {
	case *graphPanButton:
		buttons = append(buttons, object)
	case *fyne.Container:
		for _, child := range object.Objects {
			buttons = append(buttons, graphPanButtons(child)...)
		}
	case *widget.Card:
		buttons = append(buttons, graphPanButtons(object.Content)...)
	case *container.Scroll:
		buttons = append(buttons, graphPanButtons(object.Content)...)
	}
	return buttons
}

func TestGraphPanButtonsSynchronizeAndShareUndoHistory(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	buttons := graphPanButtons(ui.tabs.Items[0].Content)
	if len(buttons) != 2*len(graphVisibilityOptions) {
		t.Fatalf("expected left/right buttons on every graph, got %d", len(buttons))
	}
	for index, chart := range []*damageChart{ui.outgoing, ui.outgoingLogistics, ui.incoming, ui.incomingLogistics, ui.ewar, ui.mining} {
		back, forward := buttons[index*2], buttons[index*2+1]
		if back.forward || !forward.forward || back.Size().Width > 32 || back.Size().Height != chart.Size().Height {
			t.Fatal("pan controls are not narrow full-height buttons beside the graph")
		}
		driver := fyne.CurrentApp().Driver()
		chartLeft := driver.AbsolutePositionForObject(chart).X
		if driver.AbsolutePositionForObject(back).X+back.Size().Width > chartLeft || driver.AbsolutePositionForObject(forward).X < chartLeft+chart.Size().Width {
			t.Fatal("pan controls overlap the plot instead of sitting beside it")
		}
	}
	test.Tap(buttons[1]) // Outgoing graph: forward a quarter of one hour.
	first := graphRangeState{Start: initial.Start.Add(15 * time.Minute), End: initial.End.Add(15 * time.Minute), Preset: "Custom"}
	assertGraphHistoryState(t, ui, first, 180)
	if len(ui.graphUndo) != 1 || ui.graphUndo[0].Before.Preset != "Last hour" || ui.tableSeries[0].TotalOutgoing != 180 {
		t.Fatal("pan did not record the previous preset or update damage totals")
	}
	test.Tap(buttons[4]) // Incoming graph: move both graphs back to the original bounds.
	returned := graphRangeState{Start: initial.Start, End: initial.End, Preset: "Custom"}
	assertGraphHistoryState(t, ui, returned, 1180)
	if len(ui.graphUndo) != 2 {
		t.Fatal("each pan should add exactly one history entry")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, first, 180)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, first, 180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, returned, 1180)
	dragGraphFraction(ui.incoming, .25, .75)
	zoom := graphHistoryState(ui)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, returned, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, zoom, 30)
	if ui.graphAverageWindow == 0 || ui.incoming.data.AverageWindow != 10*time.Second {
		t.Fatal("panning or undo changed graph formatting")
	}
	ui.updateRange()
	assertGraphHistoryState(t, ui, zoom, 30)
	if len(ui.graphUndo) != 3 {
		t.Fatal("refresh discarded panning history")
	}
}

func TestGraphPanRoundsSymmetricallyAndKeepsWidth(t *testing.T) {
	for _, item := range []struct {
		width, shift time.Duration
	}{{80 * time.Second, 20 * time.Second}, {30 * time.Second, 8 * time.Second}, {5 * time.Second, time.Second}, {time.Second, time.Second}} {
		t.Run(item.width.String(), func(t *testing.T) {
			ui, start := newGraphHistoryTestUI(t)
			ui.applyGraphRange(start, start.Add(item.width))
			before := graphHistoryState(ui)
			ui.panGraphRange(true)
			if !ui.view.Start.Equal(before.Start.Add(item.shift)) || ui.view.End.Sub(ui.view.Start) != item.width {
				t.Fatal("forward pan used the wrong step or resized the window")
			}
			ui.panGraphRange(false)
			if graphHistoryState(ui) != before {
				t.Fatal("opposite pans did not return to exactly the same window")
			}
		})
	}
}

func TestGraphPanCanCrossEmptyRangesAndReplaceRedoBranch(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	buttons := graphPanButtons(ui.tabs.Items[0].Content)
	for count := 0; count < 4; count++ {
		test.Tap(buttons[5])
	}
	if !ui.view.Start.Equal(start.Add(time.Hour)) || ui.view.End.Sub(ui.view.Start) != time.Hour || len(ui.outgoing.data.Series) != 0 || len(ui.tableSeries) != 0 {
		t.Fatal("panning into an empty range clamped the window or retained old data")
	}
	if buttons[0].Disabled() || buttons[1].Disabled() {
		t.Fatal("empty range disabled the way back to damage data")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	if ui.view.Series[0].TotalOutgoing != 150 {
		t.Fatal("undo did not restore damage after an empty window")
	}
	test.Tap(buttons[0])
	branch := graphHistoryState(ui)
	if len(ui.graphRedo) != 0 {
		t.Fatal("new pan kept the abandoned redo branch")
	}
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	if graphHistoryState(ui) != branch {
		t.Fatal("redo escaped onto an abandoned panning branch")
	}
}
