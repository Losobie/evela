package main

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (chart *damageChart) MouseIn(event *desktop.MouseEvent) { chart.MouseMoved(event) }
func (chart *damageChart) MouseOut() {
	chart.pan = nil
	if chart.hoverDot != nil {
		chart.hoverDot.Hide()
	}
	chart.hovered = false
	if chart.hoverGroup != nil {
		chart.hoverGroup.leave(chart)
		return
	}
	if chart.cursor != nil {
		chart.cursor.Hide()
	}
}
func (chart *damageChart) MouseMoved(event *desktop.MouseEvent) {
	chart.hoverPosition, chart.hovered = event.Position, true
	if chart.pan != nil {
		if event.Button == desktop.MouseButtonSecondary {
			chart.updatePan(event.Position)
			return
		}
		chart.pan = nil // A release outside this widget must not leave a stuck pan.
	}
	if chart.drag != nil {
		return
	}
	if chart.hoverGroup != nil {
		chart.hoverGroup.source = chart
		chart.hoverGroup.refresh()
	} else {
		chart.updateHover(event.Position)
	}
}

// Recompute from the pointer position so refreshes inspect the new data and
// time axis even when the mouse has not moved.
func (chart *damageChart) updateHover(position fyne.Position) {
	chart.updateHoverReadout(position, nil)
}

// A shared inspection time bypasses this graph's nearest-hit search so all
// graphs describe the same instant while retaining their own values and units.
func (chart *damageChart) updateHoverReadout(position fyne.Position, inspectionTime *time.Time) (time.Time, bool) {
	if chart.hoverDot != nil {
		chart.hoverDot.Hide()
	}
	chart.mu.RLock()
	data := chart.data
	palette := chart.palette
	selected := chart.selectedSeries
	chart.mu.RUnlock()
	if (inspectionTime == nil && len(data.Series) == 0 && len(data.Markers) == 0) || data.Bucket <= 0 || !data.End.After(data.Start) || chart.hover == nil {
		return time.Time{}, false
	}
	w := chart.Size().Width - chartPlotLeft - data.plotRight()
	if w <= 0 {
		return time.Time{}, false
	}
	x := math.Max(0, math.Min(float64(w), float64(position.X-chartPlotLeft)))
	elapsed := x / float64(w) * data.End.Sub(data.Start).Seconds()
	bin := int(elapsed / data.Bucket.Seconds())
	if inspectionTime != nil {
		bin = int(inspectionTime.Sub(data.Start) / data.Bucket)
	}
	bin = min(bin, int((data.End.Sub(data.Start)-1)/data.Bucket))
	targetLine := chartLineHover{series: -1, hit: -1}
	if inspectionTime == nil {
		targetLine = closestChartLine(data, chart.Size(), position)
	}
	if !data.IndividualHits && selected != nil {
		for index, series := range data.Series {
			if seriesKey(series) == *selected {
				if point, ok := chartSeriesPositionAtX(data, chart.Size(), index, position.X); ok {
					targetLine = chartLineHover{series: index, hit: -1, position: point, found: true}
				}
				break
			}
		}
	}
	type reading struct {
		name      string
		value     float64
		color     color.NRGBA
		unit      string
		preferred bool
	}
	var readings []reading
	hoverTime := data.Start.Add(time.Duration(bin) * data.Bucket)
	if data.IndividualHits {
		hoverTime = data.Start.Add(time.Duration(math.Floor(elapsed)) * time.Second)
		nearest, distance, found := int64(0), float64(5), false
		target := float64(data.Start.Unix()) + elapsed
		if inspectionTime != nil {
			nearest, found = inspectionTime.Unix(), true
		} else if targetLine.found {
			nearest, found = data.Series[targetLine.series].Hits[targetLine.hit].Second, true
		} else {
			for _, series := range data.Series {
				index := sort.Search(len(series.Hits), func(i int) bool { return float64(series.Hits[i].Second) >= target })
				for _, candidate := range []int{index - 1, index} {
					if candidate < 0 || candidate >= len(series.Hits) {
						continue
					}
					second := series.Hits[candidate].Second
					delta := math.Abs(float64(second)-target) / data.End.Sub(data.Start).Seconds() * float64(w)
					if delta <= distance {
						nearest, distance, found = second, delta, true
					}
				}
			}
		}
		if found {
			hoverTime = time.Unix(nearest, 0).UTC()
			for seriesIndex, series := range data.Series {
				index := sort.Search(len(series.Hits), func(i int) bool { return series.Hits[i].Second >= nearest })
				for ; index < len(series.Hits) && series.Hits[index].Second == nearest; index++ {
					hit := series.Hits[index]
					name := series.Name
					if hit.Character != "" && hit.Character != name {
						name += " · " + hit.Character
					}
					if hit.Detail != "" {
						name += " · " + hit.Detail
					}
					preferred := targetLine.found && seriesIndex == targetLine.series && index == targetLine.hit
					if selected != nil {
						preferred = seriesKey(series) == *selected
					}
					readings = append(readings, reading{name, hit.Damage, series.Color, series.Unit, preferred})
				}
			}
		}
	} else {
		for seriesIndex, series := range data.Series {
			valueBin := bin
			if targetLine.found && seriesIndex == targetLine.series && len(series.Values) > 0 {
				valueBin = min(valueBin, len(series.Values)-1)
			}
			if valueBin >= 0 && valueBin < len(series.Values) {
				name := series.Name
				count := series.MemberCount
				if bin < len(series.MemberCounts) {
					count = series.MemberCounts[bin]
				}
				if count > 1 {
					name = fmt.Sprintf("%s (%d)", name, count)
				}
				preferred := targetLine.found && seriesIndex == targetLine.series
				if selected != nil {
					preferred = seriesKey(series) == *selected
				}
				readings = append(readings, reading{name, series.Values[valueBin], series.Color, series.Unit, preferred})
			}
		}
	}
	if inspectionTime != nil {
		hoverTime = *inspectionTime
	}
	sort.SliceStable(readings, func(i, j int) bool {
		if readings[i].preferred != readings[j].preferred {
			return readings[i].preferred
		}
		return readings[i].value > readings[j].value
	})
	if targetLine.found && chart.hoverDot != nil {
		if !data.IndividualHits {
			if point, ok := chartSeriesPositionAtX(data, chart.Size(), targetLine.series, position.X); ok {
				targetLine.position = point
			}
		}
		// Individual-hit readouts still use the actual hit's timestamp/amount;
		// the indicator itself shares the cursor's horizontal position.
		targetLine.position.X = chartPlotLeft + float32(x)
		chart.hoverDot.FillColor = palette.seriesColor(data.Series[targetLine.series].Color)
		chart.hoverDot.StrokeColor = palette.background
		chart.hoverDot.Move(targetLine.position.Subtract(fyne.NewPos(4, 4)))
		chart.hoverDot.Show()
		chart.hoverDot.Refresh()
	}
	chart.hover.Text = hoverTime.Format("Jan 02 15:04:05") + " EVE time"
	for _, marker := range data.Markers {
		markerX := float64(marker.At.Sub(data.Start)) / float64(data.End.Sub(data.Start)) * float64(w)
		if math.Abs(markerX-x) <= 4 {
			chart.hover.Text += " · Target invulnerable at " + marker.At.UTC().Format("15:04:05") + " EVE time: " + marker.Label
			break
		}
	}
	for _, label := range chart.hoverReadings {
		label.Hide()
	}
	for i, reading := range readings {
		if i == 3 {
			break
		}
		rate, unit := shortNumber(reading.value), "DPS"
		if data.RateUnit != "" {
			unit = data.RateUnit
		}
		if data.IndividualHits {
			unit = "damage"
			if data.HitUnit != "" {
				unit = data.HitUnit
			}
			if reading.value < 1000 {
				rate = fmt.Sprintf("%g", reading.value)
			}
		}
		if data.AverageWindow > 0 {
			if reading.value < 1000 {
				rate = fmt.Sprintf("%.1f", reading.value)
			}
			if data.RateUnit != "" && reading.value > 0 && reading.value < 0.1 {
				rate = fmt.Sprintf("%.3g", reading.value)
			}
			unit += " (" + data.AverageWindow.String() + " avg)"
		}
		if reading.unit != "" {
			unit = reading.unit
			if !data.IndividualHits {
				unit += "/s"
			}
			if data.AverageWindow > 0 {
				unit += " (" + data.AverageWindow.String() + " avg)"
			}
		}
		label := chart.hoverReadings[i]
		label.Text = fmt.Sprintf("· %s: %s %s", reading.name, rate, unit)
		label.Color = palette.seriesColor(reading.color)
		label.Show()
	}
	chart.hoverClip.Refresh()
	chart.cursor.Position1 = fyne.NewPos(float32(x)+chartPlotLeft, chartPlotTop)
	chart.cursor.Position2 = fyne.NewPos(float32(x)+chartPlotLeft, chart.Size().Height-chartPlotBottom)
	chart.cursor.Show()
	chart.cursor.Refresh()
	return hoverTime, true
}

func (chart *damageChart) setHoverHint(text string) {
	if chart.hoverDot != nil {
		chart.hoverDot.Hide()
	}
	if chart.hover == nil {
		return
	}
	chart.hover.Text = text
	for _, label := range chart.hoverReadings {
		label.Hide()
	}
	chart.hoverClip.Refresh()
}
