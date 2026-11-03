package main

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
)

// Keep the time beneath the pointer shared, rather than its pixel position:
// logistics charts with a second axis have a narrower plot area.
type chartHoverGroup struct {
	charts []*damageChart
	source *damageChart
}

func linkChartHovers(charts ...*damageChart) {
	group := &chartHoverGroup{charts: charts}
	for _, chart := range charts {
		chart.hoverGroup = group
	}
}

func (group *chartHoverGroup) leave(chart *damageChart) {
	if group.source == chart {
		group.source = nil
	}
	group.refresh()
}

func (group *chartHoverGroup) refresh() {
	at, active := group.hoverTime()
	inspectionTime := at.Truncate(time.Second)
	if active {
		if selectedTime, ok := group.source.updateHoverReadout(group.source.hoverPosition, nil); ok {
			inspectionTime = selectedTime
		} else {
			// Empty source graphs still share the pointer's timestamp.
			group.source.updateHoverReadout(group.source.hoverPosition, &inspectionTime)
		}
	}
	for _, chart := range group.charts {
		if chart.cursor == nil {
			continue
		}
		chart.mu.RLock()
		data := chart.data
		chart.mu.RUnlock()
		width := chart.Size().Width - chartPlotLeft - data.plotRight()
		if !active || chart.drag != nil || chart.pan != nil || width <= 0 || !data.End.After(data.Start) || at.Before(data.Start) || at.After(data.End) {
			chart.cursor.Hide()
			if chart.drag == nil && chart.pan == nil {
				chart.setHoverHint(data.hoverHint())
			}
			continue
		}
		x := chartPlotLeft + float32(float64(at.Sub(data.Start))/float64(data.End.Sub(data.Start)))*width
		if chart != group.source {
			chart.updateHoverReadout(fyne.NewPos(x, chartPlotTop), &inspectionTime)
		}
		chart.cursor.Position1 = fyne.NewPos(x, chartPlotTop)
		chart.cursor.Position2 = fyne.NewPos(x, chart.Size().Height-chartPlotBottom)
		chart.cursor.Show()
		chart.cursor.Refresh()
	}
}

func (group *chartHoverGroup) hoverTime() (time.Time, bool) {
	chart := group.source
	if chart == nil || !chart.hovered || chart.drag != nil || chart.pan != nil {
		return time.Time{}, false
	}
	chart.mu.RLock()
	data := chart.data
	chart.mu.RUnlock()
	width := chart.Size().Width - chartPlotLeft - data.plotRight()
	if width <= 0 || !data.End.After(data.Start) {
		return time.Time{}, false
	}
	fraction := math.Max(0, math.Min(1, float64(chart.hoverPosition.X-chartPlotLeft)/float64(width)))
	return data.Start.Add(time.Duration(fraction * float64(data.End.Sub(data.Start)))), true
}
