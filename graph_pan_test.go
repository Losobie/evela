package main

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func panGraphFraction(chart *damageChart, from, to float32) {
	width := chart.Size().Width - chartPlotLeft - chart.plotRight()
	a, b := chartPlotLeft+width*from, chartPlotLeft+width*to
	chart.MouseDown(testMouseEvent(a, 80, desktop.MouseButtonSecondary))
	chart.MouseMoved(testMouseEvent(b, 80, desktop.MouseButtonSecondary))
	chart.MouseUp(testMouseEvent(b, 80, desktop.MouseButtonSecondary))
}

func TestRightDragPanUpdatesAllGraphsAndOneUndoStep(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	charts := []*damageChart{ui.outgoing, ui.outgoingLogistics, ui.incoming, ui.incomingLogistics, ui.ewar, ui.mining}
	for _, chart := range charts {
		ui.restoreGraphRange(initial)
		ui.clearGraphRangeHistory()
		width := chart.Size().Width - chartPlotLeft - chart.plotRight()
		if chart.OnPanRange == nil {
			t.Fatal("graph has no pan callback")
		}
		a, b, c := chartPlotLeft+width*.75, chartPlotLeft+width*.625, chartPlotLeft+width*.5
		chart.MouseDown(testMouseEvent(a, 80, desktop.MouseButtonSecondary))
		chart.MouseMoved(testMouseEvent(b, 80, desktop.MouseButtonSecondary))
		if !ui.view.Start.Equal(initial.Start.Add(450*time.Second)) || chart.pan == nil {
			t.Fatal("right-move events did not pan live, or redraw cancelled gesture")
		}
		chart.MouseMoved(testMouseEvent(c, 80, desktop.MouseButtonSecondary))
		want := graphRangeState{Start: initial.Start.Add(15 * time.Minute), End: initial.End.Add(15 * time.Minute), Preset: "Custom"}
		assertGraphHistoryState(t, ui, want, 180)
		if chart.selection.Visible() || len(ui.graphUndo) != 1 || chart.pan == nil {
			t.Fatal("pan zoomed, highlighted a selection, or split history")
		}
		if !ui.mining.data.Start.Equal(want.Start) {
			t.Fatal("mining was not synchronized")
		}
		chart.MouseUp(testMouseEvent(c, 80, desktop.MouseButtonSecondary))
		chart.DragEnd()
		if chart.pan != nil || len(ui.graphUndo) != 1 {
			t.Fatal("release left pan active or duplicated history")
		}
		sendGraphShortcut(ui, &fyne.ShortcutUndo{})
		assertGraphHistoryState(t, ui, initial, 1180)
		sendGraphShortcut(ui, &fyne.ShortcutRedo{})
		assertGraphHistoryState(t, ui, want, 180)
		panGraphFraction(chart, .5, .75)
		if !ui.view.Start.Equal(initial.Start) || ui.view.End.Sub(ui.view.Start) != time.Hour || len(ui.graphUndo) != 2 {
			t.Fatal("reverse pan resized range or did not start a separate undo step")
		}
	}
	// Without side buttons, each chart spans the width of its card's content.
	for _, graph := range graphVisibilityOptions {
		card := ui.graphCards[graph.id]
		var chart *damageChart
		switch graph.id {
		case graphOutgoing:
			chart = ui.outgoing
		case graphIncoming:
			chart = ui.incoming
		case graphOutgoingLogistics:
			chart = ui.outgoingLogistics
		case graphIncomingLogistics:
			chart = ui.incomingLogistics
		case graphIncomingEWar:
			chart = ui.ewar
		case graphMining:
			chart = ui.mining
		}
		if math.Abs(float64(chart.Size().Width-card.Content.Size().Width)) > 1 {
			t.Fatal("graph is still narrowed by side controls")
		}
	}
}

func TestRightDragPanEmptyRangesAndRedoBranch(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	for i := 0; i < 4; i++ {
		panGraphFraction(ui.incoming, .75, .5)
	}
	if !ui.view.Start.Equal(start.Add(time.Hour)) || len(ui.outgoing.data.Series) != 0 || len(ui.tableSeries) != 0 {
		t.Fatal("pan did not reach an empty range")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	if ui.view.Series[0].TotalOutgoing != 150 {
		t.Fatal("undo did not restore data")
	}
	panGraphFraction(ui.outgoing, .5, .75)
	if len(ui.graphRedo) != 0 {
		t.Fatal("new pan retained old redo branch")
	}
	before := graphHistoryState(ui)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	if graphHistoryState(ui) != before {
		t.Fatal("redo followed abandoned branch")
	}
}

func TestRightDragPanCancellationAndIgnoredInput(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	chart := ui.outgoing
	initial := graphHistoryState(ui)
	width := chart.Size().Width - chartPlotLeft - chart.plotRight()
	for _, position := range []fyne.Position{fyne.NewPos(10, 80), fyne.NewPos(chartPlotLeft+10, 10)} {
		chart.MouseDown(testMouseEvent(position.X, position.Y, desktop.MouseButtonSecondary))
		chart.MouseMoved(testMouseEvent(chartPlotLeft+width*.3, 80, desktop.MouseButtonSecondary))
		chart.MouseUp(testMouseEvent(chartPlotLeft+width*.3, 80, desktop.MouseButtonSecondary))
		if graphHistoryState(ui) != initial {
			t.Fatal("pan began outside plot")
		}
	}
	a := chartPlotLeft + width*.5
	chart.MouseDown(testMouseEvent(a, 80, desktop.MouseButtonSecondary))
	chart.MouseMoved(testMouseEvent(a+2, 160, desktop.MouseButtonSecondary))
	if graphHistoryState(ui) != initial {
		t.Fatal("click jitter/vertical movement panned")
	}
	scrollGraph(chart, .5, -25)
	if graphHistoryState(ui) != initial {
		t.Fatal("wheel interrupted right drag")
	}
	chart.MouseMoved(testMouseEvent(a-width*.25, 80, desktop.MouseButtonSecondary))
	panned := graphHistoryState(ui)
	chart.MouseOut()
	chart.MouseMoved(testMouseEvent(a+width*.25, 80, 0))
	if chart.pan != nil || graphHistoryState(ui) != panned {
		t.Fatal("leaving graph left a stuck pan")
	}
	// External navigation cancels the captured time axis.
	chart.MouseDown(testMouseEvent(a, 80, desktop.MouseButtonSecondary))
	ui.applyGraphRange(start, start.Add(30*time.Minute))
	chart.MouseMoved(testMouseEvent(a+width*.25, 80, desktop.MouseButtonSecondary))
	if chart.pan != nil || ui.view.End.Sub(ui.view.Start) != 30*time.Minute {
		t.Fatal("external range change did not cancel pan")
	}
	chart.MouseUp(testMouseEvent(a, 80, desktop.MouseButtonSecondary))
	// Resizing invalidates the original pixel scale as well.
	chart.MouseDown(testMouseEvent(a, 80, desktop.MouseButtonSecondary))
	chart.Resize(fyne.NewSize(chart.Size().Width+100, chart.Size().Height))
	if chart.pan != nil {
		t.Fatal("resizing did not cancel pan")
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
