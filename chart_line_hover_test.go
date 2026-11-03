package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

func newLineHoverTestChart(t *testing.T) (*damageChart, fyne.Window, plotData) {
	t.Helper()
	app := test.NewTempApp(t)
	chart := newDamageChart()
	window := app.NewWindow("Inspect any graph line")
	t.Cleanup(window.Close)
	window.SetContent(chart)
	window.Resize(fyne.NewSize(1100, 360))
	window.Show()
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	data := plotData{Start: start, End: start.Add(8 * time.Second), Bucket: time.Second}
	for i, value := range []float64{100, 80, 60, 40, 20} {
		name := fmt.Sprintf("Line %d", i+1)
		data.Series = append(data.Series, plotSeries{Name: name, Color: characterColor(name), Values: []float64{value, value, value, value, value, value, value, value}})
	}
	chart.setData(data)
	return chart, window, data
}

func hoverLinePosition(chart *damageChart, fraction float32, series plotSeries, value float64) fyne.Position {
	w, h := chart.Size().Width-chartPlotLeft-chart.plotRight(), chart.Size().Height-chartPlotTop-chartPlotBottom
	return fyne.NewPos(chartPlotLeft+w*fraction, chartPlotTop+h*float32(1-chart.data.seriesFraction(series, value)))
}

func TestHoveredLineOutsideTopThreeIsFirstAndMarked(t *testing.T) {
	chart, window, data := newLineHoverTestChart(t)
	for _, choice := range []string{appearanceDark, appearanceLight} {
		fyne.CurrentApp().Preferences().SetString(appearancePreference, choice)
		applySavedAppearance(fyne.CurrentApp())
		chart.Refresh()
		position := hoverLinePosition(chart, .4, data.Series[4], 20)
		chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
		for i, want := range []string{"Line 5: 20 DPS", "Line 1: 100 DPS", "Line 2: 80 DPS"} {
			if !strings.Contains(chart.hoverReadings[i].Text, want) {
				t.Fatalf("reading %d: %s, want %s", i, chart.hoverReadings[i].Text, want)
			}
		}
		center := chart.hoverDot.Position().Add(fyne.NewPos(4, 4))
		if !chart.hoverDot.Visible() || math.Abs(float64(center.X-position.X)) > 1 || math.Abs(float64(center.Y-position.Y)) > 1 || chart.hoverDot.FillColor != currentChartPalette().seriesColor(data.Series[4].Color) {
			t.Fatal("hover dot is missing, misplaced or has the wrong series color")
		}
		if prefix := os.Getenv("EVELA_LINE_HOVER_CAPTURE"); prefix != "" {
			captureWindowPreview(t, window, prefix+"."+choice+".png")
		}
		chart.MouseMoved(testMouseEvent(position.X, chartPlotTop+1, 0))
		if chart.hoverDot.Visible() || !strings.Contains(chart.hoverReadings[0].Text, "Line 1") {
			t.Fatal("moving away did not restore top-three order")
		}
		chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
		chart.MouseOut()
		if chart.hoverDot.Visible() {
			t.Fatal("dot remained after leaving graph")
		}
	}
}

func TestHoveredLineUsesSlopeDualAxisAndFinalSegment(t *testing.T) {
	chart, _, data := newLineHoverTestChart(t)
	data.Series[4].Values = []float64{10, 30}
	data.Series[4].Unit = "GJ"
	data.SecondaryUnit = "GJ"
	data.SecondaryMaximum = 300
	chart.setData(data)
	// Halfway between samples: hit-testing must use the sloping line, not the
	// bucket's starting value or the HP axis shared by the other series.
	position := hoverLinePosition(chart, 1.0/16, data.Series[4], 20)
	chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
	if !chart.hoverDot.Visible() || !strings.Contains(chart.hoverReadings[0].Text, "Line 5: 10 GJ/s") {
		t.Fatal("slope or secondary-axis hover missed the lower series")
	}
	position = hoverLinePosition(chart, .8, data.Series[4], 30)
	chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
	if !chart.hoverDot.Visible() || !strings.Contains(chart.hoverReadings[0].Text, "Line 5: 30 GJ/s") {
		t.Fatal("extended final segment could not be inspected")
	}
	chart.MouseDown(testMouseEvent(position.X, position.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(position, position.Add(fyne.NewPos(20, 0))))
	if chart.hoverDot.Visible() {
		t.Fatal("dot remained over selection preview")
	}
	chart.DragEnd()
}

func TestHoveredIndividualHitPromotesExactHit(t *testing.T) {
	chart, _, data := newLineHoverTestChart(t)
	data.IndividualHits = true
	for i := range data.Series {
		data.Series[i].Hits = []plotHit{{Second: data.Start.Unix() + 2, Damage: float64(100 - i*20)}}
	}
	data.Series[4].Hits = append(data.Series[4].Hits, plotHit{Second: data.Start.Unix() + 2, Damage: 10, Character: "Small hit pilot"})
	chart.setData(data)
	position := hoverLinePosition(chart, .25, data.Series[4], 10)
	chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
	if !chart.hoverDot.Visible() || !strings.Contains(chart.hoverReadings[0].Text, "Small hit pilot: 10 damage") {
		t.Fatal("lower simultaneous hit did not come first")
	}
	if chart.hover.Text != "Oct 17 02:00:02 EVE time" {
		t.Fatal("hovered hit lost its exact timestamp")
	}
}

func TestLinkedHoverPreservesSelectedLineAndClearsStaleDot(t *testing.T) {
	chart, window, data := newLineHoverTestChart(t)
	other := newDamageChart()
	window.SetContent(container.NewVBox(chart, other))
	window.Resize(fyne.NewSize(1100, 600))
	linkChartHovers(chart, other)
	other.setData(data)
	position := hoverLinePosition(chart, .4, data.Series[4], 20)
	chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
	other.Refresh()
	if !chart.hoverDot.Visible() || !strings.Contains(chart.hoverReadings[0].Text, "Line 5") || other.hoverDot.Visible() || !strings.Contains(other.hoverReadings[0].Text, "Line 1") {
		t.Fatal("shared refresh overwrote source selection or selected an unrelated line")
	}
	if chart.hover.Text != other.hover.Text {
		t.Fatal("linked hover timestamps diverged")
	}
	data.Series = nil
	chart.setData(data)
	if chart.hoverDot.Visible() || chart.hoverReadings[0].Visible() {
		t.Fatal("empty data retained dot/readout")
	}
}
