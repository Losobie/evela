package main

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

var _ fyne.Scrollable = (*damageChart)(nil)

func (chart *damageChart) Scrolled(event *fyne.ScrollEvent) {
	if chart.OnWheelZoom == nil || chart.drag != nil || chart.pan != nil {
		return
	}
	delta := float64(event.Scrolled.DY)
	if delta == 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return
	}
	width := chart.Size().Width - chartPlotLeft - chart.plotRight()
	position := event.Position
	if event.AbsolutePosition != (fyne.Position{}) {
		// Resolve the window pointer against this chart on every wheel event,
		// including when it sits inside cards and tabs.
		position = event.AbsolutePosition.Subtract(fyne.CurrentApp().Driver().AbsolutePositionForObject(chart))
	}
	if width <= 0 || position.X < chartPlotLeft || position.X > chartPlotLeft+width || position.Y < chartPlotTop || position.Y > chart.Size().Height-chartPlotBottom {
		return
	}
	chart.mu.RLock()
	start, end := chart.data.Start, chart.data.End
	chart.mu.RUnlock()
	seconds := float64(end.Sub(start) / time.Second)
	if seconds < 1 {
		return
	}
	// Fyne's Windows desktop driver scales one wheel notch to 25 units.
	// Preserve fractional movement and bound accelerated events to four steps.
	steps := math.Max(-4, math.Min(4, delta/25))
	maximum := float64(int64(math.MaxInt64) / int64(time.Second))
	zoomed := math.Max(1, math.Min(maximum, math.Round(seconds*math.Pow(0.8, steps))))
	// Whole-second fields should still respond at small windows/deltas.
	if delta > 0 && zoomed >= seconds && seconds > 1 {
		zoomed = seconds - 1
	} else if delta < 0 && zoomed <= seconds && seconds < maximum {
		zoomed = seconds + 1
	}
	if zoomed == seconds {
		return
	}
	// Keep the time under the pointer at the same horizontal fraction of the
	// new range. Switching sides between zoom-out and zoom-in therefore pans.
	anchor := (float64(position.X) - float64(chartPlotLeft)) / float64(width)
	shift := time.Duration(math.Round((seconds-zoomed)*anchor)) * time.Second
	newStart := start.Add(shift)
	chart.OnWheelZoom(newStart, newStart.Add(time.Duration(zoomed)*time.Second))
	// Keep the colored reading and cursor under the pointer after the redraw.
	chart.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: position, AbsolutePosition: event.AbsolutePosition}})
}
