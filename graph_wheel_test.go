package main

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestGraphWheelUsesWindowPointerAndPansBetweenAnchors(t *testing.T) {
	for _, item := range []struct {
		name        string
		outAt, inAt float32
		shift       time.Duration
	}{
		{"forward", .2, .8, 15 * time.Second},
		{"backward", .8, .2, -15 * time.Second},
		{"same left anchor", .2, .2, 0},
		{"same right anchor", .8, .8, 0},
	} {
		t.Run(item.name, func(t *testing.T) {
			ui, start := newGraphHistoryTestUI(t)
			ui.snapshot.Logs[0].Points = append(ui.snapshot.Logs[0].Points, damagePoint{Second: start.Add(50 * time.Second).Unix(), Outgoing: 50, Incoming: 5})
			ui.applyGraphRange(start, start.Add(100*time.Second))
			chart := ui.outgoing
			scrollAt := func(fraction, delta float32) {
				t.Helper()
				width := chart.Size().Width - chartPlotLeft - chartPlotRight
				position := fyne.NewPos(chartPlotLeft+width*fraction, 80)
				absolute := fyne.CurrentApp().Driver().AbsolutePositionForObject(chart).Add(position)
				anchor := float64((position.X - chartPlotLeft) / width)
				before := ui.view.Start.Add(time.Duration(float64(ui.view.End.Sub(ui.view.Start)) * anchor))
				// The window pointer is authoritative even if the supplied local
				// coordinates disagree with the graph's position in its containers.
				chart.Scrolled(&fyne.ScrollEvent{
					PointEvent: fyne.PointEvent{AbsolutePosition: absolute, Position: fyne.NewPos(chartPlotLeft+width/2, 80)},
					Scrolled:   fyne.NewDelta(0, delta),
				})
				after := ui.view.Start.Add(time.Duration(float64(ui.view.End.Sub(ui.view.Start)) * anchor))
				if math.Abs(float64(after.Sub(before))) > float64(time.Second/2+time.Millisecond) {
					t.Fatalf("time beneath pointer moved from %s to %s", before, after)
				}
				if !chart.cursor.Visible() || math.Abs(float64(chart.cursor.Position1.X-position.X)) > .01 {
					t.Fatal("hover cursor does not match the zoom anchor")
				}
			}
			scrollAt(item.outAt, -25)
			if ui.view.End.Sub(ui.view.Start) != 125*time.Second {
				t.Fatal("wheel down did not zoom out")
			}
			scrollAt(item.inAt, 25)
			if !ui.view.Start.Equal(start.Add(item.shift)) || !ui.view.End.Equal(start.Add(100*time.Second+item.shift)) {
				t.Fatalf("changing zoom anchors did not pan %s: %+v", item.name, graphHistoryState(ui))
			}
			for _, graph := range []*damageChart{ui.outgoing, ui.incoming} {
				if !graph.data.Start.Equal(ui.view.Start) || !graph.data.End.Equal(ui.view.End) {
					t.Fatal("pointer-anchored zoom did not synchronize the graphs")
				}
			}
		})
	}
}

func scrollGraph(chart *damageChart, fraction float32, delta float32) {
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	chart.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(chartPlotLeft+width*fraction, 80)}, Scrolled: fyne.NewDelta(0, delta)})
}

func TestGraphWheelZoomDirectionAnchorAndLimits(t *testing.T) {
	for _, item := range []struct {
		name       string
		seconds    int
		fraction   float32
		delta      float32
		start, end int
		want       bool
	}{
		{"zoom in", 100, .25, 25, 5, 85, true},
		{"zoom out", 100, .25, -25, -6, 119, true},
		{"left anchor", 100, 0, -25, 0, 125, true},
		{"right anchor", 100, 1, -25, -25, 100, true},
		{"fractional wheel", 100, .25, 1, 0, 99, true},
		{"accelerated wheel", 100, .25, 100000, 15, 56, true},
		{"minimum", 1, .5, 25, 0, 0, false},
		{"leave minimum", 1, 0, -25, 0, 2, true},
		{"zero", 100, .5, 0, 0, 0, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			application := test.NewTempApp(t)
			chart := newDamageChart()
			window := application.NewWindow("Wheel zoom")
			defer window.Close()
			window.SetPadded(false)
			window.SetContent(chart)
			window.Resize(fyne.NewSize(1076, 260))
			window.Show()
			start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
			chart.setData(plotData{Start: start, End: start.Add(time.Duration(item.seconds) * time.Second), Bucket: time.Second})
			calls := 0
			chart.OnWheelZoom = func(from, to time.Time) {
				calls++
				if !from.Equal(start.Add(time.Duration(item.start)*time.Second)) || !to.Equal(start.Add(time.Duration(item.end)*time.Second)) {
					t.Fatalf("unexpected zoom: %s — %s", from, to)
				}
			}
			scrollGraph(chart, item.fraction, item.delta)
			if (calls == 1) != item.want || calls > 1 {
				t.Fatalf("wheel produced %d changes, expected change=%v", calls, item.want)
			}
		})
	}
}

func TestGraphWheelGroupsOneGestureAndSeparatesPauseUndoRedo(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	at := time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)
	for _, chart := range []*damageChart{ui.outgoing, ui.incoming} {
		chart.OnWheelZoom = func(start, end time.Time) { ui.applyGraphRangeAction(start, end, true, at) }
	}
	scrollGraph(ui.outgoing, .5, -25)
	at = at.Add(100 * time.Millisecond)
	scrollGraph(ui.incoming, .5, -25)
	at = at.Add(100 * time.Millisecond)
	scrollGraph(ui.outgoing, .5, -25)
	zoom := graphHistoryState(ui)
	assertGraphHistoryState(t, ui, zoom, 1180)
	if len(ui.graphUndo) != 1 || ui.graphUndo[0].Before != initial || ui.graphUndo[0].After != zoom {
		t.Fatal("continuous wheel events did not form one complete undo action")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, zoom, 1180)
	at = at.Add(100 * time.Millisecond)
	scrollGraph(ui.outgoing, .5, -25)
	second := graphHistoryState(ui)
	if len(ui.graphUndo) != 2 {
		t.Fatal("wheel after redo merged into the redone action")
	}
	at = at.Add(graphWheelGroupInterval + time.Millisecond)
	scrollGraph(ui.incoming, .5, -25)
	third := graphHistoryState(ui)
	if len(ui.graphUndo) != 3 {
		t.Fatal("wheel gesture after a pause did not create a new action")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, second, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, zoom, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, second, 1180)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, third, 1180)
	if ui.graphAverageWindow == 0 || ui.incoming.data.AverageWindow != 10*time.Second {
		t.Fatal("wheel zoom changed graph formatting")
	}
}

func TestGraphWheelKeepsPanAndDragSeparateAndReplacesRedo(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	initial := graphHistoryState(ui)
	at := time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)
	for _, chart := range []*damageChart{ui.outgoing, ui.incoming} {
		chart.OnWheelZoom = func(start, end time.Time) { ui.applyGraphRangeAction(start, end, true, at) }
	}
	scrollGraph(ui.outgoing, .5, -25)
	wheel := graphHistoryState(ui)
	ui.panGraphRange(false)
	pan := graphHistoryState(ui)
	dragGraphFraction(ui.incoming, .1, .9)
	drag := graphHistoryState(ui)
	at = at.Add(100 * time.Millisecond)
	scrollGraph(ui.outgoing, .5, -25)
	if len(ui.graphUndo) != 4 {
		t.Fatal("wheel merged with another kind of graph navigation")
	}
	for _, want := range []graphRangeState{drag, pan, wheel, initial} {
		sendGraphShortcut(ui, &fyne.ShortcutUndo{})
		if graphHistoryState(ui) != want {
			t.Fatalf("undo skipped an action: got %+v, want %+v", graphHistoryState(ui), want)
		}
	}
	at = at.Add(100 * time.Millisecond)
	scrollGraph(ui.incoming, .5, 25)
	branch := graphHistoryState(ui)
	if len(ui.graphRedo) != 0 || len(ui.graphUndo) != 1 {
		t.Fatal("wheel after undo retained an abandoned branch")
	}
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	if graphHistoryState(ui) != branch {
		t.Fatal("redo restored an abandoned wheel gesture")
	}
}

func TestGraphWheelIgnoresMarginsDraggingAndOverlays(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	before := graphHistoryState(ui)
	chart := ui.outgoing
	for _, event := range []*fyne.ScrollEvent{
		{PointEvent: fyne.PointEvent{Position: fyne.NewPos(5, 80)}, Scrolled: fyne.NewDelta(0, 25)},
		{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 5)}, Scrolled: fyne.NewDelta(0, 25)},
		{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 80)}, Scrolled: fyne.NewDelta(25, 0)},
	} {
		chart.Scrolled(event)
	}
	from, to := fyne.NewPos(chartPlotLeft+50, 80), fyne.NewPos(chartPlotLeft+200, 80)
	chart.MouseDown(testMouseEvent(from.X, from.Y, 1))
	chart.Dragged(testDragEvent(from, to))
	scrollGraph(chart, .5, 25)
	if chart.drag == nil || !chart.selection.Visible() {
		t.Fatal("wheel interrupted an in-progress drag selection")
	}
	chart.cancelRangeDrag()
	ui.openESISettings()
	scrollGraph(chart, .5, 25)
	ui.esiDialog.Hide()
	test.Tap(ui.graphFormatButton)
	scrollGraph(chart, .5, 25)
	if graphHistoryState(ui) != before || len(ui.graphUndo) != 0 {
		t.Fatal("wheel outside the plot, during a drag, or behind an overlay changed the range")
	}
}
