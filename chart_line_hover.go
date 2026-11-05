package main

import (
	"math"
	"sort"

	"fyne.io/fyne/v2"
)

const chartLineHoverRadius = 6.0

type chartLineHover struct {
	series, hit int
	position    fyne.Position
	found       bool
}

// Interpolate the rendered line at the cursor's time, rather than projecting
// the pointer onto its nearest point (which can move sideways on steep lines).
func chartSeriesPositionAtX(data plotData, size fyne.Size, seriesIndex int, cursorX float32) (fyne.Position, bool) {
	w, h := float64(size.Width-chartPlotLeft-data.plotRight()), float64(size.Height-chartPlotTop-chartPlotBottom)
	if w <= 0 || h <= 0 || data.Bucket <= 0 || !data.End.After(data.Start) || seriesIndex < 0 || seriesIndex >= len(data.Series) {
		return fyne.Position{}, false
	}
	series := data.Series[seriesIndex]
	if data.IndividualHits || len(series.Values) == 0 {
		return fyne.Position{}, false
	}
	x := math.Max(0, math.Min(w, float64(cursorX-chartPlotLeft)))
	sample := x / w * data.End.Sub(data.Start).Seconds() / data.Bucket.Seconds()
	bin := min(int(sample), len(series.Values)-1)
	value := series.Values[bin]
	if bin+1 < len(series.Values) {
		value += (series.Values[bin+1] - value) * (sample - float64(bin))
	}
	return fyne.NewPos(chartPlotLeft+float32(x), chartPlotTop+float32(h*(1-data.seriesFraction(series, value)))), true
}

// Hit-test the actual rendered geometry. Only inspect samples within the
// pointer's horizontal tolerance rather than walking every plotted sample.
func closestChartLine(data plotData, size fyne.Size, pointer fyne.Position) chartLineHover {
	result := chartLineHover{series: -1, hit: -1}
	w, h := float64(size.Width-chartPlotLeft-data.plotRight()), float64(size.Height-chartPlotTop-chartPlotBottom)
	x, y := float64(pointer.X-chartPlotLeft), float64(pointer.Y-chartPlotTop)
	if w <= 0 || h <= 0 || x < 0 || x > w || y < 0 || y > h || !data.End.After(data.Start) || data.Bucket <= 0 {
		return result
	}
	best := chartLineHoverRadius * chartLineHoverRadius
	consider := func(series, hit int, px, py float64) {
		distance := (x-px)*(x-px) + (y-py)*(y-py)
		if distance <= best {
			best = distance
			result = chartLineHover{series: series, hit: hit, position: fyne.NewPos(chartPlotLeft+float32(px), chartPlotTop+float32(py)), found: true}
		}
	}
	duration := data.End.Sub(data.Start).Seconds()
	step := data.Bucket.Seconds() / duration * w
	for seriesIndex, series := range data.Series {
		if data.IndividualHits {
			left := float64(data.Start.Unix()) + (x-chartLineHoverRadius)/w*duration
			right := float64(data.Start.Unix()) + (x+chartLineHoverRadius)/w*duration
			index := sort.Search(len(series.Hits), func(i int) bool { return float64(series.Hits[i].Second) >= left })
			for ; index < len(series.Hits) && float64(series.Hits[index].Second) <= right; index++ {
				hit := series.Hits[index]
				if hit.Second < data.Start.Unix() || hit.Second >= data.End.Unix() {
					continue
				}
				px := float64(hit.Second-data.Start.Unix()) / duration * w
				consider(seriesIndex, index, px, h*(1-data.seriesFraction(series, hit.Damage)))
			}
			continue
		}
		last := len(series.Values) - 1
		if last < 0 {
			continue
		}
		first := min(last, max(0, int(math.Floor((x-chartLineHoverRadius)/step))))
		end := min(last, int(math.Floor((x+chartLineHoverRadius)/step)))
		for bin := first; bin <= end; bin++ {
			x1, x2 := float64(bin)*step, w
			y1 := h * (1 - data.seriesFraction(series, series.Values[bin]))
			y2 := y1
			if bin < last {
				x2 = float64(bin+1) * step
				y2 = h * (1 - data.seriesFraction(series, series.Values[bin+1]))
			}
			dx, dy := x2-x1, y2-y1
			fraction := 0.0
			if length := dx*dx + dy*dy; length > 0 {
				fraction = math.Max(0, math.Min(1, ((x-x1)*dx+(y-y1)*dy)/length))
			}
			consider(seriesIndex, -1, x1+fraction*dx, y1+fraction*dy)
		}
	}
	return result
}
