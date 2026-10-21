package main

import (
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestLinearDamageScale(t *testing.T) {
	data := plotData{Maximum: 999}
	for _, point := range []struct{ value, fraction float64 }{{0, 0}, {249.75, .25}, {499.5, .5}, {999, 1}} {
		if got := data.fraction(point.value); math.Abs(got-point.fraction) > 1e-10 {
			t.Fatalf("%g DPS appears at %g of the plot, want %g", point.value, got, point.fraction)
		}
		if got := data.valueAt(point.fraction); math.Abs(got-point.value) > 1e-10 {
			t.Fatalf("axis at %g is labeled %g DPS, want %g", point.fraction, got, point.value)
		}
	}
	if data.fraction(-1) != 0 || data.fraction(2000) != 1 || data.valueAt(-1) != 0 || math.Abs(data.valueAt(2)-999) > 1e-10 {
		t.Fatal("out-of-range values were not clamped to the plot")
	}
	if data.fraction(9) != 9.0/999 || data.valueAt(0.5) != 499.5 {
		t.Fatal("linear scale changed")
	}
	if got := (plotData{}).fraction(0); got != 0 || math.IsNaN(got) {
		t.Fatal("empty chart has an invalid coordinate")
	}
}

func TestLinearScaleRasterAndDPS(t *testing.T) {
	application := test.NewTempApp(t)
	chart := newDamageChart()
	window := application.NewWindow("Scale raster")
	defer window.Close()
	window.SetContent(chart)
	window.Resize(fyne.NewSize(700, 300))
	window.Show()
	start := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	ink := color.NRGBA{255, 0, 0, 255}
	chart.data = plotData{Start: start, End: start.Add(2 * time.Second), Bucket: time.Second, Maximum: 999,
		Series: []plotSeries{{Name: "Pilot", Color: ink, Values: []float64{9, 9}}}}
	linear := chart.renderPlot(101, 121)
	if linear.At(50, 119) != (color.RGBA{255, 0, 0, 255}) {
		t.Fatal("linear data did not render near the baseline")
	}
	renderer := test.WidgetRenderer(chart).(*chartRenderer)
	renderer.Refresh()
	if renderer.yLabels[0].Text != "999" || renderer.yLabels[4].Text != "0" {
		t.Fatal("axis labels are not in DPS")
	}
	chart.MouseMoved(testMouseEvent(chartPlotLeft+50, 80, 0))
	if !strings.Contains(chart.hoverReadings[0].Text, "Pilot: 9 DPS") {
		t.Fatalf("hover should show raw DPS: %s", chart.hoverReadings[0].Text)
	}
}

func TestLegacyLogarithmicPreferencesUseLinearGraphs(t *testing.T) {
	application := test.NewTempApp(t)
	application.Preferences().SetBool("graph.format.logarithmic", true)
	window := application.NewWindow("Legacy scale settings")
	defer window.Close()
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot"}, Logs: []*parsedLog{{Character: "Pilot", Points: []damagePoint{{Second: start.Unix(), Outgoing: 100, Incoming: 50}}}}}
	ui.selected["Pilot"] = true
	ui.applyGraphRange(start, start.Add(time.Minute))
	charts := []*damageChart{ui.outgoing, ui.incoming}
	for _, chart := range charts {
		if math.Abs(chart.data.fraction(chart.data.Maximum/4)-.25) > 1e-10 || math.Abs(chart.data.valueAt(.5)-chart.data.Maximum/2) > 1e-10 {
			t.Fatal("an older logarithmic preference overrode linear scaling")
		}
	}
	for _, item := range ui.graphFormatMenu().Items {
		if item.Label == "Linear" || strings.Contains(strings.ToLower(item.Label), "logarithmic") {
			t.Fatal("format menu still offers a scale choice")
		}
	}
}

func TestGraphFormatMenuSynchronizesGraphsGroupingAndRefresh(t *testing.T) {
	application := test.NewTempApp(t)
	application.Preferences().SetBool(graphAveragePreference, false)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Graph format preview")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A", "Pilot B"}, First: start, Last: start.Add(time.Minute), Logs: []*parsedLog{
		{Character: "Pilot A", Points: []damagePoint{{Second: start.Unix() + 10, Outgoing: 9, Incoming: 3}, {Second: start.Unix() + 20, Outgoing: 99, Incoming: 9}, {Second: start.Unix() + 30, Outgoing: 999, Incoming: 99}}},
		{Character: "Pilot B", Points: []damagePoint{{Second: start.Unix() + 10, Outgoing: 19, Incoming: 4}, {Second: start.Unix() + 20, Outgoing: 199, Incoming: 19}, {Second: start.Unix() + 30, Outgoing: 1999, Incoming: 199}}},
	}}
	ui.selected["Pilot A"], ui.selected["Pilot B"] = true, true
	ui.bucketSelect.SetSelected("1 second")
	ui.applyGraphRange(start, start.Add(time.Minute))
	applyTestShipRecords(ui, []shipRecord{{CharacterName: "Pilot A", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()}, {CharacterName: "Pilot B", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()}})
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	choose := func(label string) {
		t.Helper()
		for _, item := range append(ui.graphFormatMenu().Items, ui.graphOptionsMenu().Items...) {
			if item.Label == label {
				item.Action()
				return
			}
		}
		t.Fatalf("missing format option %q", label)
	}
	checked := func(label string) bool {
		for _, item := range append(ui.graphFormatMenu().Items, ui.graphOptionsMenu().Items...) {
			if item.Label == label {
				return item.Checked
			}
		}
		return false
	}
	if !checked("Character") || len(ui.outgoing.data.Series) != 2 {
		t.Fatal("default format should be linear with separate characters")
	}
	beforeSize, beforePosition := ui.outgoing.Size(), application.Driver().AbsolutePositionForObject(ui.outgoing)
	summary := ui.summary.Text
	choose("Ship type (ship history)")
	if !ui.shipGrouping() || len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Values[10] != 28 || len(ui.incoming.data.Series) != 2 || ui.incoming.data.Series[0].Values[10] != 3 || ui.tableSeries[0].TotalOutgoing != 3324 || ui.summary.Text != summary {
		t.Fatal("ship format did not combine damage correctly")
	}
	if !strings.Contains(ui.graphFormatLabel.Text, "Linear") || !strings.Contains(ui.graphFormatLabel.Text, "ship type") {
		t.Fatal("active format was not displayed")
	}
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+float32(10.1/60)*(ui.outgoing.Size().Width-chartPlotLeft-chartPlotRight), 80, 0))
	if label := ui.outgoing.hoverReadings[0]; !strings.Contains(label.Text, "Loki · Pilot B: 19 damage") || label.Color != characterColor("Loki") {
		t.Fatalf("ship-type hit hover lost its character or color: %s", label.Text)
	}
	ui.selected["Pilot B"] = false
	ui.updateRange() // Automatic log refresh.
	if ui.outgoing.data.Series[0].Values[10] != 9 || !checked("Ship type (ship history)") {
		t.Fatal("refresh lost graph format or selection")
	}
	if label := ui.outgoing.hoverReadings[0]; !strings.Contains(label.Text, "Loki · Pilot A: 9 damage") {
		t.Fatalf("stationary hover retained the old count after deselection: %s", label.Text)
	}
	ui.applyGraphRange(start.Add(5*time.Second), start.Add(45*time.Second))
	historyLength := len(ui.graphUndo)
	choose("Character")
	if ui.shipGrouping() || len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Name != "Pilot A" || len(ui.graphUndo) != historyLength {
		t.Fatal("switching format lost character selection or drag history")
	}
	ui.undoGraphRange()
	if !ui.view.Start.Equal(start) || !ui.view.End.Equal(start.Add(time.Minute)) {
		t.Fatal("range undo changed formatting or failed after a format change")
	}
	window.Canvas().Capture()
	if ui.outgoing.Size() != beforeSize || application.Driver().AbsolutePositionForObject(ui.outgoing) != beforePosition {
		t.Fatal("changing graph format moved or resized the graph")
	}
	ui.groupSelect.SetSelected("Ship history")
	if !checked("Ship type (ship history)") {
		t.Fatal("sidebar grouping and format menu disagree")
	}
	beforeAverage := ui.summary.Text
	historyLength = len(ui.graphUndo)
	choose("10-second DPS average")
	if !checked("10-second DPS average") || ui.outgoing.data.AverageWindow != 10*time.Second || ui.incoming.data.AverageWindow != 10*time.Second || math.Abs(ui.outgoing.data.Series[0].Values[10]-0.9) > 1e-10 || math.Abs(ui.incoming.data.Series[0].Values[10]-0.3) > 1e-10 {
		t.Fatal("average option did not update both graphs with the correct rolling rates")
	}
	if ui.summary.Text != beforeAverage || len(ui.graphUndo) != historyLength || !strings.Contains(ui.graphFormatLabel.Text, "10-second average") {
		t.Fatal("averaging changed totals, zoom history or formatting")
	}
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+float32(10.5/60)*(ui.outgoing.Size().Width-chartPlotLeft-chartPlotRight), 80, 0))
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, "0.9 DPS (10s avg)") {
		t.Fatalf("hover did not identify the fractional averaged rate: %s", ui.outgoing.hoverReadings[0].Text)
	}
	choose("Character")
	if ui.outgoing.data.Series[0].Name != "Pilot A" || math.Abs(ui.outgoing.data.Series[0].Values[10]-0.9) > 1e-10 {
		t.Fatal("per-character mode lost averaging")
	}
	choose("10-second DPS average")
	if checked("10-second DPS average") || ui.outgoing.data.AverageWindow != 0 || ui.outgoing.data.Series[0].Values[10] != 9 {
		t.Fatal("turning averaging off did not restore raw bucket DPS")
	}
	choose("10-second DPS average")
	ui.snapshot.Logs[0].Points = append(ui.snapshot.Logs[0].Points, damagePoint{Second: start.Unix() + 10, Outgoing: 90})
	ui.updateRange()
	if !checked("10-second DPS average") || math.Abs(ui.outgoing.data.Series[0].Values[10]-9.9) > 1e-10 || ui.tableSeries[0].TotalOutgoing != 1197 {
		t.Fatal("refresh lost averaging, used stale averaged data or changed raw totals")
	}
	test.Tap(ui.graphFormatButton)
	if window.Canvas().Overlays().Top() == nil {
		t.Fatal("Format options button did not open its dropdown")
	}
	if path := os.Getenv("EVELA_FORMAT_CAPTURE"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGraphFormatPreferencesRestoreBeforeLoadingLogs(t *testing.T) {
	application := test.NewTempApp(t)
	newUI := func() *analyzerUI {
		window := application.NewWindow("Saved graph format")
		t.Cleanup(window.Close)
		return newAnalyzerUI(window)
	}
	choose := func(ui *analyzerUI, label string) {
		t.Helper()
		for _, item := range append(ui.graphFormatMenu().Items, ui.graphOptionsMenu().Items...) {
			if item.Label == label {
				item.Action()
				return
			}
		}
		t.Fatalf("missing format option %q", label)
	}
	first := newUI()
	if !first.graphAverage10s || first.outgoing.data.AverageWindow != dpsAverageWindow || first.incoming.data.AverageWindow != dpsAverageWindow {
		t.Fatal("new users did not get averaged DPS on both graphs by default")
	}
	choose(first, "Ship class (ship history)")
	// A new UI reads the same saved user preferences, as on the next launch.
	restored := newUI()
	if !restored.graphAverage10s || !restored.shipClassGrouping() || !strings.Contains(restored.graphFormatLabel.Text, "By ship class") || !strings.Contains(restored.graphFormatLabel.Text, "10-second average") {
		t.Fatal("startup lost the saved grouping, average or format label")
	}
	for _, chart := range []*damageChart{restored.outgoing, restored.incoming} {
		if chart.data.AverageWindow != 10*time.Second {
			t.Fatal("saved format was not applied to both graphs before loading logs")
		}
	}
	for _, item := range append(restored.graphFormatMenu().Items, restored.graphOptionsMenu().Items...) {
		want := item.Label == "10-second DPS average" || item.Label == "Ship class (ship history)" || strings.HasPrefix(item.Label, "Show ")
		if item.Checked != want {
			t.Fatalf("restored menu checkmark for %q = %v, want %v", item.Label, item.Checked, want)
		}
	}
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	restored.snapshot = &logSnapshot{Characters: []string{"A", "B"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 10, Incoming: 2}}},
		{Character: "B", Points: []damagePoint{{Second: start.Unix(), Outgoing: 20, Incoming: 4}}},
	}}
	restored.selected["A"], restored.selected["B"] = true, true
	restored.applyGraphRange(start, start.Add(20*time.Second))
	applyTestShipRecords(restored, []shipRecord{
		{CharacterName: "A", TypeID: 1, TypeName: "Loki", ClassName: "Strategic Cruiser", CheckedAt: time.Now()},
		{CharacterName: "B", TypeID: 2, TypeName: "Tengu", ClassName: "Strategic Cruiser", CheckedAt: time.Now()},
	})
	if len(restored.outgoing.data.Series) != 1 || restored.outgoing.data.Series[0].Name != "Strategic Cruiser" || restored.outgoing.data.Series[0].Values[0] != 3 || len(restored.incoming.data.Series) != 2 || math.Abs(restored.incoming.data.Series[0].Values[0]-.2) > 1e-10 {
		t.Fatal("first loaded data did not use the restored class grouping and average")
	}
	choose(restored, "10-second DPS average")
	choose(restored, "Character")
	reset := newUI()
	if reset.graphAverage10s || reset.shipGrouping() {
		t.Fatal("turning format options off was not saved")
	}
	reset.groupSelect.SetSelected("Ship history")
	if next := newUI(); !next.shipGrouping() || next.shipClassGrouping() {
		t.Fatal("sidebar grouping was not saved with the format preferences")
	}
	application.Preferences().SetString(graphGroupingPreference, "obsolete-grouping")
	if next := newUI(); next.shipGrouping() || next.groupSelect.Selected != "Characters" {
		t.Fatal("unknown saved grouping did not fall back to per-character mode")
	}
}
