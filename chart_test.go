package main

import (
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func testMouseEvent(x, y float32, button desktop.MouseButton) *desktop.MouseEvent {
	return &desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, y)}, Button: button}
}

func testDragEvent(from, to fyne.Position) *fyne.DragEvent {
	return &fyne.DragEvent{PointEvent: fyne.PointEvent{Position: to}, Dragged: fyne.NewDelta(to.X-from.X, to.Y-from.Y)}
}

func TestDamageChartDragSelection(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		from, to   fyne.Position
		button     desktop.MouseButton
		want       bool
		start, end time.Duration
	}{
		{"forward", fyne.NewPos(312, 80), fyne.NewPos(812, 80), desktop.MouseButtonPrimary, true, 25 * time.Second, 75 * time.Second},
		{"reverse", fyne.NewPos(812, 80), fyne.NewPos(312, 80), desktop.MouseButtonPrimary, true, 25 * time.Second, 75 * time.Second},
		{"clamp right", fyne.NewPos(312, 80), fyne.NewPos(1500, 400), desktop.MouseButtonPrimary, true, 25 * time.Second, 100 * time.Second},
		{"clamp left", fyne.NewPos(812, 80), fyne.NewPos(-200, -20), desktop.MouseButtonPrimary, true, 0, 75 * time.Second},
		{"tiny movement", fyne.NewPos(312, 80), fyne.NewPos(315, 80), desktop.MouseButtonPrimary, false, 0, 0},
		{"vertical movement", fyne.NewPos(312, 80), fyne.NewPos(312, 200), desktop.MouseButtonPrimary, false, 0, 0},
		{"axis margin", fyne.NewPos(10, 80), fyne.NewPos(812, 80), desktop.MouseButtonPrimary, false, 0, 0},
		{"header", fyne.NewPos(312, 10), fyne.NewPos(812, 80), desktop.MouseButtonPrimary, false, 0, 0},
		{"right button", fyne.NewPos(312, 80), fyne.NewPos(812, 80), desktop.MouseButtonSecondary, false, 0, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			application := test.NewTempApp(t)
			chart := newDamageChart()
			window := application.NewWindow("Chart selection")
			defer window.Close()
			window.SetPadded(false)
			window.SetContent(chart)
			window.Resize(fyne.NewSize(1076, 260))
			window.Show()
			start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
			chart.setData(plotData{Start: start, End: start.Add(100 * time.Second), Bucket: time.Second})
			var gotStart, gotEnd time.Time
			calls := 0
			chart.OnRangeSelected = func(from, to time.Time) { calls++; gotStart, gotEnd = from, to }
			chart.MouseDown(testMouseEvent(testCase.from.X, testCase.from.Y, testCase.button))
			chart.Dragged(testDragEvent(testCase.from, testCase.to))
			if testCase.want && (!chart.selection.Visible() || !chart.data.Start.Equal(start)) {
				t.Fatal("selection preview should be visible before changing the range")
			}
			chart.DragEnd()
			chart.MouseUp(testMouseEvent(testCase.to.X, testCase.to.Y, testCase.button))
			if testCase.want {
				if calls != 1 || !gotStart.Equal(start.Add(testCase.start)) || !gotEnd.Equal(start.Add(testCase.end)) {
					t.Fatalf("got %s – %s (%d callbacks)", gotStart, gotEnd, calls)
				}
			} else if calls != 0 {
				t.Fatal("click, invalid start or non-primary drag selected a range")
			}
			if chart.selection.Visible() || chart.drag != nil {
				t.Fatal("selection remained after release")
			}
		})
	}
}

func TestDamageChartSelectionRefreshAndEventOrdering(t *testing.T) {
	application := test.NewTempApp(t)
	chart := newDamageChart()
	window := application.NewWindow("Chart selection")
	defer window.Close()
	window.SetPadded(false)
	window.SetContent(chart)
	window.Resize(fyne.NewSize(1076, 260))
	window.Show()
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	data := plotData{Start: start, End: start.Add(100 * time.Second), Bucket: time.Second}
	chart.setData(data)
	calls := 0
	chart.OnRangeSelected = func(time.Time, time.Time) { calls++ }
	from, to := fyne.NewPos(312, 80), fyne.NewPos(812, 80)
	// Touch events can start without a desktop MouseDown.
	chart.Dragged(testDragEvent(from, to))
	if !strings.Contains(chart.hover.Text, "EVE time · (50 seconds) · release to zoom") {
		t.Fatalf("selection duration missing from readout: %s", chart.hover.Text)
	}
	chart.DragEnd()
	if calls != 1 {
		t.Fatal("drag without MouseDown did not select")
	}
	chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(from, to))
	chart.setData(data)
	chart.MouseOut()
	if chart.drag == nil || !chart.selection.Visible() {
		t.Fatal("refresh of unchanged time axis or mouse exit lost the selection")
	}
	// Accept the alternate MouseUp-before-DragEnd driver order exactly once.
	chart.MouseUp(testMouseEvent(to.X, to.Y, desktop.MouseButtonPrimary))
	chart.DragEnd()
	if calls != 2 {
		t.Fatal("alternate release order duplicated or lost selection")
	}
	chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(from, to))
	changed := data
	changed.End = changed.End.Add(time.Second)
	chart.setData(changed)
	chart.Dragged(testDragEvent(from, to))
	chart.DragEnd()
	if calls != 2 || chart.selection.Visible() {
		t.Fatal("changed time axis should cancel the old selection")
	}
	chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(from, to))
	chart.Resize(fyne.NewSize(1176, 260))
	chart.DragEnd()
	if calls != 2 {
		t.Fatal("resizing should cancel rather than remap a pending selection")
	}
}

func TestGraphDragSynchronizesRangesAndTotals(t *testing.T) {
	application := test.NewTempApp(t)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Drag to select time")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A", "Pilot B"}, Logs: []*parsedLog{
		{Character: "Pilot A", Points: []damagePoint{
			{Second: start.Unix(), Outgoing: 1000, Incoming: 100, OutgoingHits: 1, IncomingHits: 1},
			{Second: start.Add(30 * time.Second).Unix(), Outgoing: 30, Incoming: 3, OutgoingHits: 1, IncomingHits: 1},
			{Second: start.Add(60 * time.Second).Unix(), Outgoing: 60, Incoming: 6, OutgoingHits: 1, IncomingHits: 1},
			{Second: start.Add(90 * time.Second).Unix(), Outgoing: 90, Incoming: 9, OutgoingHits: 1, IncomingHits: 1},
		}},
		{Character: "Pilot B", Points: []damagePoint{{Second: start.Add(30 * time.Second).Unix(), Outgoing: 300, Incoming: 30, OutgoingHits: 1, IncomingHits: 1}}},
	}}
	ui.selected["Pilot A"], ui.selected["Pilot B"] = true, false
	ui.applyGraphRange(start, start.Add(100*time.Second))
	applyTestShipRecords(ui, []shipRecord{{CharacterName: "Pilot A", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()}, {CharacterName: "Pilot B", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()}})
	ui.groupSelect.SetSelected("Ship history")
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	for index, chart := range []*damageChart{ui.outgoing, ui.incoming} {
		ui.applyGraphRange(start, start.Add(100*time.Second))
		width := chart.Size().Width - chartPlotLeft - chartPlotRight
		from, to := fyne.NewPos(chartPlotLeft+width*.2, 80), fyne.NewPos(chartPlotLeft+width*.8, 80)
		chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
		chart.Dragged(testDragEvent(from, to))
		if path := os.Getenv("EVELA_DRAG_CAPTURE"); path != "" && index == 0 {
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, window.Canvas().Capture())
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		chart.DragEnd()
		if ui.rangeSelect.Selected != "Custom" {
			t.Fatal("drag did not select a custom range")
		}
		if ui.view.Start.Before(start.Add(19*time.Second)) || ui.view.Start.After(start.Add(21*time.Second)) || ui.view.End.Before(start.Add(79*time.Second)) || ui.view.End.After(start.Add(81*time.Second)) {
			t.Fatal("drag applied the wrong time interval")
		}
		if !ui.incoming.data.Start.Equal(ui.view.Start) || !ui.outgoing.data.End.Equal(ui.view.End) {
			t.Fatal("graphs are not synchronized")
		}
		if ui.view.Series[0].TotalOutgoing != 90 || ui.view.Series[0].TotalIncoming != 9 || ui.tableSeries[0].TotalOutgoing != 90 {
			t.Fatal("damage totals did not filter to selected interval")
		}
		if !ui.selected["Pilot A"] || ui.selected["Pilot B"] || !ui.shipGrouping() {
			t.Fatal("zoom changed character selection or grouping")
		}
		beforeStart, beforeEnd := ui.view.Start, ui.view.End
		ui.updateRange()
		if !ui.view.Start.Equal(beforeStart) || !ui.view.End.Equal(beforeEnd) {
			t.Fatal("refresh reset the selected interval")
		}
	}
}
