package main

import (
	"image/color"

	"fyne.io/fyne/v2"
)

func (chart *damageChart) selectSeriesAt(position fyne.Position) {
	chart.mu.Lock()
	hit := closestChartLine(chart.data, chart.Size(), position)
	var selected *chartSeriesKey
	if hit.found {
		key := seriesKey(chart.data.Series[hit.series])
		if chart.selectedSeries == nil || *chart.selectedSeries != key {
			selected = &key
		}
	}
	chart.selectedSeries = selected
	chart.mu.Unlock()
	chart.hoverPosition, chart.hovered = position, true
	chart.Refresh()
	chart.updateHover(position)
}

func (chart *damageChart) clearSeriesSelection() {
	chart.mu.Lock()
	chart.selectedSeries = nil
	chart.mu.Unlock()
	chart.Refresh()
}

func dimSeries(ink, background color.NRGBA) color.NRGBA {
	return color.NRGBA{
		uint8(.25*float64(ink.R) + .75*float64(background.R)),
		uint8(.25*float64(ink.G) + .75*float64(background.G)),
		uint8(.25*float64(ink.B) + .75*float64(background.B)), ink.A,
	}
}
