package main

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func assertSharedGraphHover(t *testing.T, ui *analyzerUI, at time.Time) {
	t.Helper()
	for name, chart := range map[string]*damageChart{
		"outgoing": ui.outgoing, "incoming": ui.incoming, "E-War": ui.ewar,
		"outgoing logistics": ui.outgoingLogistics, "incoming logistics": ui.incomingLogistics,
	} {
		if chart.cursor == nil || !chart.cursor.Visible() {
			t.Fatalf("%s has no synchronized time marker", name)
		}
		width := chart.Size().Width - chartPlotLeft - chart.data.plotRight()
		fraction := float64(chart.cursor.Position1.X-chartPlotLeft) / float64(width)
		got := chart.data.Start.Add(time.Duration(fraction * float64(chart.data.End.Sub(chart.data.Start))))
		if math.Abs(float64(got.Sub(at))) > float64(10*time.Microsecond) {
			t.Fatalf("%s highlights %s, want %s", name, got.Format(time.RFC3339Nano), at.Format(time.RFC3339Nano))
		}
		if chart.cursor.Position1.Y != chartPlotTop || chart.cursor.Position2.Y != chart.Size().Height-chartPlotBottom {
			t.Fatalf("%s marker escaped the plot", name)
		}
	}
}

func assertNoSharedGraphHover(t *testing.T, ui *analyzerUI) {
	t.Helper()
	for _, chart := range ui.outgoing.hoverGroup.charts {
		if chart.cursor.Visible() {
			t.Fatal("a graph retained its time marker after hover ended")
		}
	}
}

func TestGraphHoverLinksEveryGraphIncludingEmptyAndDualAxisPlots(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	ui.logisticsTypes[graphOutgoingLogistics].set("Capacitor transfer", true)
	ui.logisticsTypes[graphIncomingLogistics].set("Capacitor transfer", true)
	ui.renderSelection()
	if ui.outgoingLogistics.data.SecondaryUnit != "GJ" {
		t.Fatal("fixture must cover a dual-axis plot")
	}
	empty := ui.outgoing.data
	empty.Series = nil
	ui.outgoing.setData(empty)
	ui.graphsScroll.ScrollToBottom()
	for _, chart := range ui.outgoing.hoverGroup.charts {
		width := chart.Size().Width - chartPlotLeft - chart.data.plotRight()
		chart.MouseIn(testMouseEvent(chartPlotLeft+width*.40625, 80, 0))
		assertSharedGraphHover(t, ui, start.Add(3250*time.Millisecond))
		// Refreshing a chart without the pointer must retain its linked line.
		ui.incoming.Refresh()
		assertSharedGraphHover(t, ui, start.Add(3250*time.Millisecond))
		chart.MouseOut()
		assertNoSharedGraphHover(t, ui)
		ui.incoming.Refresh()
		assertNoSharedGraphHover(t, ui)
	}
	// A late MouseOut from the old graph must not erase the new source's line.
	ui.outgoing.MouseIn(testMouseEvent(chartPlotLeft+100, 80, 0))
	width := ui.ewar.Size().Width - chartPlotLeft - ui.ewar.plotRight()
	ui.ewar.MouseIn(testMouseEvent(chartPlotLeft+width/2, 80, 0))
	ui.outgoing.MouseOut()
	assertSharedGraphHover(t, ui, start.Add(4*time.Second))
}

func TestGraphHoverSurvivesFiltersResizeVisibilityAndWheelZoom(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	chart := ui.outgoingLogistics
	chart.MouseIn(testMouseEvent(chartPlotLeft+200, 80, 0))
	// Enabling capacitor adds a second axis and changes the time under this
	// stationary pointer. All graphs must adopt that new time together.
	ui.logisticsTypes[graphOutgoingLogistics].set("Capacitor transfer", true)
	ui.graphAverage10s = true
	ui.renderSelection()
	width := chart.Size().Width - chartPlotLeft - chart.plotRight()
	at := chart.data.Start.Add(time.Duration(200 / float64(width) * float64(chart.data.End.Sub(chart.data.Start))))
	assertSharedGraphHover(t, ui, at)
	ui.incoming.Resize(fyne.NewSize(ui.incoming.Size().Width-120, ui.incoming.Size().Height))
	assertSharedGraphHover(t, ui, at)

	fyne.CurrentApp().Preferences().SetBool(graphVisibilityPreference+graphIncomingEWar, false)
	ui.applyGraphVisibility()
	fyne.CurrentApp().Preferences().SetBool(graphVisibilityPreference+graphIncomingEWar, true)
	ui.applyGraphVisibility()
	at, _ = chart.hoverGroup.hoverTime()
	assertSharedGraphHover(t, ui, at)

	ui.graphsScroll.ScrollToBottom()
	position := fyne.NewPos(chartPlotLeft+(chart.Size().Width-chartPlotLeft-chart.plotRight())*.4, 80)
	absolute := fyne.CurrentApp().Driver().AbsolutePositionForObject(chart).Add(position)
	chart.Scrolled(&fyne.ScrollEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(0, 0), AbsolutePosition: absolute},
		Scrolled:   fyne.NewDelta(0, -25),
	})
	if ui.view.End.Sub(ui.view.Start) != 10*time.Second {
		t.Fatal("wheel did not zoom the scrolled logistics graph")
	}
	width = chart.Size().Width - chartPlotLeft - chart.plotRight()
	at = chart.data.Start.Add(time.Duration(float64(position.X-chartPlotLeft) / float64(width) * float64(chart.data.End.Sub(chart.data.Start))))
	assertSharedGraphHover(t, ui, at)
}

func TestGraphHoverClearsForRangeSelection(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	chart := ui.incoming
	chart.MouseIn(testMouseEvent(chartPlotLeft+100, 80, 0))
	chart.MouseDown(testMouseEvent(chartPlotLeft+100, 80, 1))
	assertNoSharedGraphHover(t, ui)
	chart.Dragged(testDragEvent(fyne.NewPos(chartPlotLeft+100, 80), fyne.NewPos(chartPlotLeft+200, 80)))
	ui.outgoing.Refresh()
	chart.Refresh()
	assertNoSharedGraphHover(t, ui)
	if !chart.selection.Visible() {
		t.Fatal("linked hover replaced the range selection")
	}
	chart.DragEnd()
	chart.MouseMoved(testMouseEvent(chartPlotLeft+100, 80, 0))
	at, _ := chart.hoverGroup.hoverTime()
	assertSharedGraphHover(t, ui, at)
	chart.MouseOut()
	assertNoSharedGraphHover(t, ui)
}
