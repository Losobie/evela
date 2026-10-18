package main

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestChartHoverColorsFollowSortedSeries(t *testing.T) {
	application := test.NewTempApp(t)
	chart := newDamageChart()
	window := application.NewWindow("Hover colors")
	defer window.Close()
	window.SetPadded(false)
	window.SetContent(chart)
	window.Resize(fyne.NewSize(1100, 300))
	window.Show()
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	red, green, blue, yellow := color.NRGBA{255, 100, 100, 255}, color.NRGBA{100, 255, 140, 255}, color.NRGBA{110, 170, 255, 255}, color.NRGBA{255, 220, 100, 255}
	data := plotData{Start: start, End: start.Add(2 * time.Second), Bucket: time.Second, Series: []plotSeries{
		{Name: "Pilot Red", Color: red, Values: []float64{30, 1}},
		{Name: "Pilot Green", Color: green, Values: []float64{10, 100}},
		{Name: "Pilot Blue", Color: blue, Values: []float64{20, 5}},
		{Name: "Pilot Yellow", Color: yellow, Values: []float64{5, 2}},
	}}
	chart.setData(data)
	muted := chart.hover.Color
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	chart.MouseMoved(testMouseEvent(chartPlotLeft+width/4, 80, 0))
	for index, want := range []struct {
		text string
		ink  color.NRGBA
	}{{"Pilot Red: 30 DPS", red}, {"Pilot Blue: 20 DPS", blue}, {"Pilot Green: 10 DPS", green}} {
		label := chart.hoverReadings[index]
		if !label.Visible() || !strings.Contains(label.Text, want.text) || label.Color != want.ink {
			t.Fatalf("reading %d has wrong text/color: %s / %v", index, label.Text, label.Color)
		}
	}
	// At the exclusive right edge, inspect the last bucket rather than dropping
	// all character readings. Their new order must retain the series colors.
	chart.MouseMoved(testMouseEvent(chartPlotLeft+width, 80, 0))
	for index, want := range []struct {
		name string
		ink  color.NRGBA
	}{{"Pilot Green", green}, {"Pilot Blue", blue}, {"Pilot Yellow", yellow}} {
		if label := chart.hoverReadings[index]; !label.Visible() || !strings.Contains(label.Text, want.name) || label.Color != want.ink {
			t.Fatalf("resorted reading %d lost its color", index)
		}
	}
	if chart.hover.Color != muted || chart.hover.Text != "Oct 17 02:00:01 UTC" {
		t.Fatal("timestamp changed color or reports the wrong bucket")
	}
	data.Logarithmic, data.AverageWindow = true, 10*time.Second
	chart.setData(data)
	chart.MouseMoved(testMouseEvent(chartPlotLeft+width, 80, 0))
	if label := chart.hoverReadings[0]; label.Color != green || !strings.Contains(label.Text, "100.0 DPS (10s avg)") {
		t.Fatal("logarithmic/averaged hover lost its color or DPS formatting")
	}
}

func TestChartHoverColorsClearForHintsAndStayInsideHeader(t *testing.T) {
	application := test.NewTempApp(t)
	chart := newDamageChart()
	window := application.NewWindow("Hover layout")
	defer window.Close()
	window.SetPadded(false)
	window.SetContent(chart)
	window.Resize(fyne.NewSize(550, 240))
	window.Show()
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	data := plotData{Start: start, End: start.Add(100 * time.Second), Bucket: time.Second,
		Series: []plotSeries{{Name: strings.Repeat("Long ship group name ", 20), Color: characterColor("Loki"), Values: []float64{100}}}}
	chart.setData(data)
	size := chart.Size()
	chart.MouseMoved(testMouseEvent(chartPlotLeft+1, 80, 0))
	window.Canvas().Capture()
	if chart.Size() != size || chart.hoverClip.Size().Width != size.Width-chartPlotLeft-chartPlotRight || chart.hoverClip.Size().Height != chartPlotTop {
		t.Fatal("long colored readout resized the plot or escaped its fixed header")
	}
	chart.MouseDown(testMouseEvent(chartPlotLeft+50, 80, 1))
	chart.Dragged(testDragEvent(fyne.NewPos(chartPlotLeft+50, 80), fyne.NewPos(chartPlotLeft+200, 80)))
	if !strings.HasPrefix(chart.hover.Text, "Select ") {
		t.Fatal("selection instructions were not restored")
	}
	for _, label := range chart.hoverReadings {
		if label.Visible() {
			t.Fatal("colored DPS reading remained over selection instructions")
		}
	}
	chart.DragEnd()
	chart.MouseMoved(testMouseEvent(chartPlotLeft+1, 80, 0))
	chart.setData(plotData{Start: data.Start, End: data.End, Bucket: time.Second})
	for _, label := range chart.hoverReadings {
		if label.Visible() {
			t.Fatal("empty refresh retained a stale character reading")
		}
	}
}

func TestChartRefreshUpdatesStationaryHover(t *testing.T) {
	application := test.NewTempApp(t)
	chart := newDamageChart()
	window := application.NewWindow("Live hover")
	defer window.Close()
	window.SetPadded(false)
	window.SetContent(chart)
	window.Resize(fyne.NewSize(1076, 260))
	window.Show()
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	red, green := characterColor("Pilot Red"), characterColor("Pilot Green")
	data := plotData{Start: start, End: start.Add(4 * time.Second), Bucket: time.Second, Series: []plotSeries{
		{Name: "Pilot Red", Color: red, Values: []float64{10, 20, 30, 40}},
		{Name: "Pilot Green", Color: green, Values: []float64{1, 2, 3, 4}},
	}}
	chart.setData(data)
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	chart.MouseIn(testMouseEvent(chartPlotLeft+width*.6, 80, 0))
	cursorPosition := chart.cursor.Position1
	if !strings.Contains(chart.hoverReadings[0].Text, "Pilot Red: 30 DPS") {
		t.Fatal("initial hover selected the wrong bucket")
	}

	// Change the values, ranking, time axis, bucket size and formatting without
	// sending another mouse event, as happens during an automatic log refresh.
	data.Start, data.End = start.Add(time.Second), start.Add(5*time.Second)
	data.Bucket, data.AverageWindow, data.Logarithmic = 2*time.Second, 10*time.Second, true
	data.Series[0].Values = []float64{50, 60}
	data.Series[1].Values = []float64{70, 80}
	chart.setData(data)
	if chart.hover.Text != "Oct 17 02:00:03 UTC" || !chart.cursor.Visible() || chart.cursor.Position1 != cursorPosition {
		t.Fatalf("refresh lost the pointer or selected time: %q", chart.hover.Text)
	}
	for index, want := range []struct {
		text string
		ink  color.NRGBA
	}{{"Pilot Green: 80.0 DPS (10s avg)", green}, {"Pilot Red: 60.0 DPS (10s avg)", red}} {
		label := chart.hoverReadings[index]
		if !label.Visible() || !strings.Contains(label.Text, want.text) || label.Color != want.ink {
			t.Fatalf("refresh left stale reading %d: %q / %v", index, label.Text, label.Color)
		}
	}
	chart.Refresh()
	if !chart.cursor.Visible() || chart.hover.Text != "Oct 17 02:00:03 UTC" {
		t.Fatal("ordinary redraw cleared the hover")
	}

	chart.MouseDown(testMouseEvent(chartPlotLeft+width*.6, 80, 1))
	chart.Dragged(testDragEvent(fyne.NewPos(chartPlotLeft+width*.6, 80), fyne.NewPos(chartPlotLeft+width*.8, 80)))
	chart.setData(data)
	if !strings.HasPrefix(chart.hover.Text, "Select ") || chart.cursor.Visible() || !chart.selection.Visible() {
		t.Fatal("refresh replaced the drag selection with hover information")
	}
	chart.DragEnd()
	chart.MouseOut()
	chart.setData(data)
	if chart.cursor.Visible() || !strings.HasPrefix(chart.hover.Text, chartHint) {
		t.Fatal("refresh restored a hover after the pointer left the graph")
	}
	for _, label := range chart.hoverReadings {
		if label.Visible() {
			t.Fatal("refresh restored readings after mouse exit")
		}
	}
}
