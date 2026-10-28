package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func assertSharedGraphHover(t *testing.T, ui *analyzerUI, at time.Time) {
	t.Helper()
	stamp := strings.Split(ui.outgoing.hoverGroup.source.hover.Text, " EVE time")[0] + " EVE time"
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
		if !strings.HasPrefix(chart.hover.Text, stamp) {
			t.Fatalf("%s readout reports %q, want shared time %q", name, chart.hover.Text, stamp)
		}
	}
}

func assertNoSharedGraphHover(t *testing.T, ui *analyzerUI) {
	t.Helper()
	for _, chart := range ui.outgoing.hoverGroup.charts {
		if chart.cursor.Visible() {
			t.Fatal("a graph retained its time marker after hover ended")
		}
		if chart.drag == nil {
			for _, reading := range chart.hoverReadings {
				if reading.Visible() {
					t.Fatal("a graph retained a value after hover ended")
				}
			}
		}
	}
}

func TestGraphHoverSharesInspectionTimeAndUpdatesEveryReadout(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	charts := []*damageChart{ui.outgoing, ui.incoming, ui.ewar, ui.outgoingLogistics, ui.incomingLogistics}
	for i, chart := range charts {
		unit := []string{"damage", "damage", "GJ", "HP", "GJ"}[i]
		hits := []plotHit{{Second: start.Add(10 * time.Second).Unix(), Damage: float64(10 + i)}}
		if chart != ui.outgoing {
			// This event is nearer the pointer than the source's 10-second hit.
			// Linked graphs must still inspect the source's selected time.
			hits = append(hits, plotHit{Second: start.Add(11 * time.Second).Unix(), Damage: 999})
		}
		hits = append(hits, plotHit{Second: start.Add(30 * time.Second).Unix(), Damage: float64(20 + i)})
		chart.setData(plotData{
			Start: start, End: start.Add(1000 * time.Second), Bucket: time.Second,
			IndividualHits: true, HitUnit: unit,
			Series: []plotSeries{{Name: "Pilot", Color: characterColor(unit), Hits: hits}},
		})
	}
	move := func(seconds float64) {
		t.Helper()
		width := ui.outgoing.Size().Width - chartPlotLeft - ui.outgoing.plotRight()
		ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+width*float32(seconds/1000), 80, 0))
	}
	check := func(second int, values []string) {
		t.Helper()
		for i, chart := range charts {
			want := start.Add(time.Duration(second)*time.Second).Format("Jan 02 15:04:05") + " EVE time"
			if chart.hover.Text != want {
				t.Fatalf("graph %d readout time = %q, want %q", i, chart.hover.Text, want)
			}
			label := chart.hoverReadings[0]
			if !label.Visible() || !strings.Contains(label.Text, values[i]) || label.Color != chart.data.Series[0].Color {
				t.Fatalf("graph %d readout = %q (%v), want %q in its series color", i, label.Text, label.Visible(), values[i])
			}
		}
	}
	move(10.9)
	check(10, []string{"10 damage", "11 damage", "12 GJ", "13 HP", "14 GJ"})
	move(30.1)
	check(30, []string{"20 damage", "21 damage", "22 GJ", "23 HP", "24 GJ"})
	data := ui.incoming.data
	data.Series[0].Hits[2].Damage = 77
	ui.incoming.setData(data)
	check(30, []string{"20 damage", "77 damage", "22 GJ", "23 HP", "24 GJ"})
	data.Series = nil
	ui.incoming.setData(data)
	if ui.incoming.hover.Text != "Oct 20 02:00:30 EVE time" || ui.incoming.hoverReadings[0].Visible() {
		t.Fatal("an empty graph retained stale values or lost the shared timestamp")
	}
	move(50.1)
	for _, chart := range charts {
		if chart.hover.Text != "Oct 20 02:00:50 EVE time" {
			t.Fatal("empty hover did not advance the shared timestamp")
		}
		for _, reading := range chart.hoverReadings {
			if reading.Visible() {
				t.Fatal("moving away from events retained an old value")
			}
		}
	}
	for i, chart := range charts {
		data := chart.data
		data.IndividualHits, data.Bucket, data.AverageWindow = false, 10*time.Second, 10*time.Second
		data.RateUnit = []string{"DPS", "DPS", "GJ/s", "HP/s", "GJ/s"}[i]
		data.Series = []plotSeries{{Name: "Pilot", Color: characterColor(data.RateUnit), Values: []float64{1, float64(20 + i)}}}
		chart.setData(data)
	}
	move(12.1)
	check(10, []string{"20.0 DPS (10s avg)", "21.0 DPS (10s avg)", "22.0 GJ/s (10s avg)", "23.0 HP/s (10s avg)", "24.0 GJ/s (10s avg)"})
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
	ui.graphAverageWindow = dpsAverageWindow
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
