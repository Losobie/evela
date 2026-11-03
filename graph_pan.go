package main

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
)

type chartPanDrag struct {
	Start, End, LastStart, LastEnd time.Time
	Width, Anchor                  float32
	Applied                        bool
}

func (chart *damageChart) beginPan(position fyne.Position) bool {
	if chart.OnPanRange == nil || !chart.beginRangeDrag(position) {
		return false
	}
	drag := chart.drag
	chart.drag = nil
	if chart.cursor != nil {
		chart.cursor.Hide()
	}
	chart.pan = &chartPanDrag{Start: drag.Start, End: drag.End, LastStart: drag.Start, LastEnd: drag.End, Width: drag.Width, Anchor: position.X}
	return true
}

func (chart *damageChart) updatePan(position fyne.Position) {
	pan := chart.pan
	if pan == nil || chart.OnPanRange == nil {
		return
	}
	delta := float64(pan.Anchor - position.X)
	if math.IsNaN(delta) || math.IsInf(delta, 0) || !pan.Applied && math.Abs(delta) < float64(chartDragThreshold) {
		return
	}
	// Drag the data with the pointer: dragging right reveals earlier times.
	seconds := math.Round(delta / float64(pan.Width) * pan.End.Sub(pan.Start).Seconds())
	limit := float64(math.MaxInt64 / int64(time.Second))
	shift := time.Duration(math.Max(-limit, math.Min(limit, seconds))) * time.Second
	start, end := pan.Start.Add(shift), pan.End.Add(shift)
	if start.Equal(pan.LastStart) && end.Equal(pan.LastEnd) {
		return
	}
	continuing := pan.Applied
	// Record the expected axis before the synchronous redraw, so setData can
	// distinguish our own pan from an external zoom, undo or range change.
	pan.LastStart, pan.LastEnd, pan.Applied = start, end, true
	chart.OnPanRange(start, end, continuing)
}

func (ui *analyzerUI) applyGraphPanRange(start, end time.Time, continuing bool) {
	if ui.window.Canvas().Overlays().Top() != nil {
		return
	}
	start, end = start.UTC().Truncate(time.Second), end.UTC().Truncate(time.Second)
	if !end.After(start) || start.Equal(ui.view.Start) && end.Equal(ui.view.End) {
		return
	}
	before := graphRangeState{Start: ui.view.Start, End: ui.view.End, Preset: ui.rangeSelect.Selected}
	if continuing && len(ui.graphUndo) > 0 && len(ui.graphRedo) == 0 {
		last := &ui.graphUndo[len(ui.graphUndo)-1]
		if !last.wheel && last.After == before {
			last.After = graphRangeState{Start: start, End: end, Preset: "Custom"}
			ui.restoreGraphRange(last.After)
			return
		}
	}
	ui.applyGraphRange(start, end)
}
