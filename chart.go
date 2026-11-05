package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

func characterColor(name string) color.NRGBA {
	// Stable across filtering, refreshes and application restarts.
	var hash uint32 = 2166136261
	for _, value := range []byte(name) {
		hash = (hash ^ uint32(value)) * 16777619
	}
	// Spread even large character rosters across the color wheel rather than
	// cycling a short palette. Keep enough brightness for the dark plot area.
	h := float64(hash%3600) / 600
	c := 0.58
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g = c, x
	case 1:
		r, g = x, c
	case 2:
		g, b = c, x
	case 3:
		g, b = x, c
	case 4:
		r, b = x, c
	case 5:
		r, b = c, x
	}
	return color.NRGBA{uint8((r + 0.38) * 255), uint8((g + 0.38) * 255), uint8((b + 0.38) * 255), 255}
}

type plotSeries struct {
	Name         string
	Unit         string
	Color        color.NRGBA
	Values       []float64
	Hits         []plotHit
	MemberCount  int
	MemberCounts []int
}
type plotData struct {
	Start, End        time.Time
	Bucket            time.Duration
	Series            []plotSeries
	Maximum           float64
	AverageWindow     time.Duration
	Markers           []plotMarker
	IndividualHits    bool
	RateUnit, HitUnit string
	EmptyText         string
	SecondaryUnit     string
	SecondaryMaximum  float64
}

func (data plotData) plotRight() float32 {
	if data.SecondaryUnit != "" {
		return 66
	}
	return chartPlotRight
}

func (chart *damageChart) plotRight() float32 {
	chart.mu.RLock()
	defer chart.mu.RUnlock()
	return chart.data.plotRight()
}

func (data plotData) seriesFraction(series plotSeries, value float64) float64 {
	if data.SecondaryUnit != "" && series.Unit == data.SecondaryUnit {
		data.Maximum = data.SecondaryMaximum
	}
	return data.fraction(value)
}

// Plot coordinates and axis labels always use a linear scale.
func (data plotData) fraction(value float64) float64 {
	if data.Maximum <= 0 {
		return 0
	}
	value = math.Max(0, math.Min(data.Maximum, value))
	return value / data.Maximum
}

func (data plotData) valueAt(fraction float64) float64 {
	fraction = math.Max(0, math.Min(1, fraction))
	return data.Maximum * fraction
}

type damageChart struct {
	palette chartPalette
	widget.BaseWidget
	mu              sync.RWMutex
	data            plotData
	selectedSeries  *chartSeriesKey
	hover           *canvas.Text
	hoverReadings   [3]*canvas.Text
	hoverClip       *container.Scroll
	hoverPosition   fyne.Position
	hovered         bool
	hoverGroup      *chartHoverGroup
	cursor          *canvas.Line
	hoverDot        *canvas.Circle
	selection       *canvas.Rectangle
	drag            *chartRangeDrag
	pan             *chartPanDrag
	dragBlocked     bool
	OnRangeSelected func(time.Time, time.Time)
	OnWheelZoom     func(time.Time, time.Time)
	OnPanRange      func(time.Time, time.Time, bool)
}

type chartRangeDrag struct {
	Start, End             time.Time
	Width, Anchor, Current float32
	Moved                  bool
}

const (
	chartPlotLeft      float32 = 62
	chartPlotTop       float32 = 25
	chartPlotRight     float32 = 14
	chartPlotBottom    float32 = 27
	chartDragThreshold float32 = 4
	chartHint                  = "Wheel/left-drag: zoom · Right-drag: pan · Undo: Ctrl+Z / Back · Redo: Ctrl+Y / Forward · EVE time"
)

var _ fyne.Draggable = (*damageChart)(nil)
var _ desktop.Mouseable = (*damageChart)(nil)

func newDamageChart() *damageChart {
	chart := &damageChart{palette: currentChartPalette()}
	chart.ExtendBaseWidget(chart)
	return chart
}

func (chart *damageChart) setData(data plotData) {
	if chart.pan != nil && (!chart.pan.LastStart.Equal(data.Start) || !chart.pan.LastEnd.Equal(data.End)) {
		chart.pan = nil
	}
	// A refresh with the same time axis may update the data during a drag.
	// A changed axis cancels it so the gesture cannot select the wrong times.
	if chart.drag != nil && (!chart.drag.Start.Equal(data.Start) || !chart.drag.End.Equal(data.End)) {
		chart.cancelRangeDrag()
		chart.dragBlocked = true
	}
	for _, series := range data.Series {
		maximum := &data.Maximum
		if data.SecondaryUnit != "" && series.Unit == data.SecondaryUnit {
			maximum = &data.SecondaryMaximum
		}
		if data.IndividualHits {
			for _, hit := range series.Hits {
				*maximum = math.Max(*maximum, hit.Damage)
			}
		} else {
			for _, value := range series.Values {
				if value > *maximum {
					*maximum = value
				}
			}
		}
	}
	if data.Maximum <= 0 {
		data.Maximum = 1
	} else {
		data.Maximum *= 1.08
	}
	if data.SecondaryMaximum <= 0 {
		data.SecondaryMaximum = 1
	} else {
		data.SecondaryMaximum *= 1.08
	}
	chart.mu.Lock()
	if chart.selectedSeries != nil {
		found := false
		for _, series := range data.Series {
			found = found || seriesKey(series) == *chart.selectedSeries
		}
		if !found {
			chart.selectedSeries = nil
		}
	}
	chart.data = data
	chart.mu.Unlock()
	chart.Refresh()
}

func (chart *damageChart) CreateRenderer() fyne.WidgetRenderer {
	palette := currentChartPalette()
	muted := palette.text
	chart.hover = canvas.NewText("Move over the graph to inspect damage", muted)
	chart.hover.TextSize = 11
	hoverObjects := []fyne.CanvasObject{chart.hover}
	for index := range chart.hoverReadings {
		reading := canvas.NewText("", muted)
		reading.TextSize = 11
		reading.Hide()
		chart.hoverReadings[index] = reading
		hoverObjects = append(hoverObjects, reading)
	}
	// Clip long readouts to the existing header instead of resizing the graph
	// or allowing text to spill beyond the plot.
	chart.hoverClip = container.NewHScroll(container.NewHBox(hoverObjects...))
	chart.hoverClip.Direction = container.ScrollNone
	chart.cursor = canvas.NewLine(palette.cursor)
	chart.cursor.Hide()
	chart.hoverDot = canvas.NewCircle(palette.text)
	chart.hoverDot.StrokeColor = palette.background
	chart.hoverDot.StrokeWidth = 1.5
	chart.hoverDot.Resize(fyne.NewSize(8, 8))
	chart.hoverDot.Hide()
	chart.selection = canvas.NewRectangle(palette.selection)
	chart.selection.StrokeColor = palette.selectionStroke
	chart.selection.StrokeWidth = 1
	chart.selection.Hide()
	r := &chartRenderer{chart: chart, plot: canvas.NewRaster(chart.renderPlot)}
	r.empty = canvas.NewText("No damage in this range for the selected characters", muted)
	r.empty.TextSize = 13
	r.objects = []fyne.CanvasObject{r.plot, chart.selection, chart.cursor, chart.hoverDot, chart.hoverClip, r.empty}
	for i := 0; i < 5; i++ {
		y, x := canvas.NewText("", muted), canvas.NewText("", muted)
		y.TextSize, x.TextSize = 11, 11
		r.yLabels = append(r.yLabels, y)
		r.xLabels = append(r.xLabels, x)
		r.objects = append(r.objects, y, x)
		right := canvas.NewText("", muted)
		right.TextSize = 11
		r.rightLabels = append(r.rightLabels, right)
		r.objects = append(r.objects, right)
	}
	r.leftUnit, r.rightUnit = canvas.NewText("", muted), canvas.NewText("", muted)
	r.leftUnit.TextSize, r.rightUnit.TextSize = 11, 11
	r.objects = append(r.objects, r.leftUnit, r.rightUnit)
	r.Refresh()
	return r
}

type chartRenderer struct {
	chart               *damageChart
	plot                *canvas.Raster
	empty               *canvas.Text
	yLabels, xLabels    []*canvas.Text
	objects             []fyne.CanvasObject
	rightLabels         []*canvas.Text
	leftUnit, rightUnit *canvas.Text
}

func (r *chartRenderer) Layout(size fyne.Size) {
	w, h := fyne.Max(size.Width-chartPlotLeft-r.chart.plotRight(), 1), fyne.Max(size.Height-chartPlotTop-chartPlotBottom, 1)
	if r.chart.pan != nil && r.chart.pan.Width != w {
		r.chart.pan = nil
	}
	if r.chart.drag != nil && r.chart.drag.Width != w {
		r.chart.cancelRangeDrag()
		r.chart.dragBlocked = true
	}
	r.plot.Move(fyne.NewPos(chartPlotLeft, chartPlotTop))
	r.plot.Resize(fyne.NewSize(w, h))
	r.chart.hoverClip.Move(fyne.NewPos(chartPlotLeft, 0))
	r.chart.hoverClip.Resize(fyne.NewSize(w, chartPlotTop))
	r.empty.Move(fyne.NewPos(80, 32))
	r.leftUnit.Move(fyne.NewPos(2, 0))
	r.rightUnit.Move(fyne.NewPos(chartPlotLeft+w+6, 0))
	for i := 0; i < 5; i++ {
		y := float32(i)*h/4 + 19
		r.yLabels[i].Move(fyne.NewPos(2, y))
		r.rightLabels[i].Move(fyne.NewPos(chartPlotLeft+w+6, y))
		x := chartPlotLeft + float32(i)*w/4
		if i == 4 {
			x -= r.xLabels[i].MinSize().Width
		} else if i > 0 {
			x -= r.xLabels[i].MinSize().Width / 2
		}
		r.xLabels[i].Move(fyne.NewPos(x, h+28))
	}
	if r.chart.hoverGroup != nil {
		r.chart.hoverGroup.refresh()
	}
}
func (r *chartRenderer) MinSize() fyne.Size {
	return fyne.NewSize(550, 175)
}
func (r *chartRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chartRenderer) Destroy()                     {}
func (r *chartRenderer) Refresh() {
	palette := currentChartPalette()
	r.chart.mu.Lock()
	r.chart.palette = palette
	r.chart.mu.Unlock()
	r.chart.hover.Color = palette.text
	r.empty.Color = palette.text
	r.leftUnit.Color = palette.text
	r.rightUnit.Color = palette.text
	for _, labels := range [][]*canvas.Text{r.xLabels, r.yLabels, r.rightLabels} {
		for _, label := range labels {
			label.Color = palette.text
		}
	}
	r.chart.cursor.StrokeColor = palette.cursor
	r.chart.selection.FillColor = palette.selection
	r.chart.selection.StrokeColor = palette.selectionStroke

	r.chart.mu.RLock()
	data := r.chart.data
	r.chart.mu.RUnlock()
	hasDamage := false
	for _, series := range data.Series {
		if data.IndividualHits {
			hasDamage = hasDamage || len(series.Hits) > 0
			continue
		}
		for _, value := range series.Values {
			if value > 0 {
				hasDamage = true
				break
			}
		}
	}
	if hasDamage {
		r.empty.Hide()
	} else {
		r.empty.Show()
	}
	r.empty.Text = "No damage in this range for the selected characters"
	if data.EmptyText != "" {
		r.empty.Text = data.EmptyText
	}
	r.empty.Refresh()
	r.leftUnit.Hide()
	r.rightUnit.Hide()
	if data.SecondaryUnit != "" {
		r.leftUnit.Text, r.rightUnit.Text = data.HitUnit, data.SecondaryUnit
		if !data.IndividualHits {
			r.leftUnit.Text += "/s"
			r.rightUnit.Text += "/s"
		}
		r.leftUnit.Show()
		r.rightUnit.Show()
		r.leftUnit.Refresh()
		r.rightUnit.Refresh()
	}
	for i := 0; i < 5; i++ {
		value := data.valueAt(float64(4-i) / 4)
		r.yLabels[i].Text = shortNumber(value)
		if value > 0 && value < 1 {
			r.yLabels[i].Text = fmt.Sprintf("%.2g", value)
		}
		r.yLabels[i].Refresh()
		r.rightLabels[i].Hide()
		if data.SecondaryUnit != "" {
			value := data.SecondaryMaximum * float64(4-i) / 4
			r.rightLabels[i].Text = shortNumber(value)
			if value > 0 && value < 1 {
				r.rightLabels[i].Text = fmt.Sprintf("%.2g", value)
			}
			r.rightLabels[i].Show()
			r.rightLabels[i].Refresh()
		}
		at := data.Start.Add(time.Duration(float64(data.End.Sub(data.Start)) * float64(i) / 4))
		layout := "15:04:05"
		if data.End.Sub(data.Start) > 24*time.Hour {
			layout = "Jan 02 15:04"
		}
		r.xLabels[i].Text = at.Format(layout)
		r.xLabels[i].Refresh()
	}
	r.Layout(r.chart.Size())
	r.chart.cursor.Hide()
	r.chart.setHoverHint(data.hoverHint())
	if r.chart.drag != nil && r.chart.drag.Moved {
		r.chart.updateRangeSelection()
	} else {
		r.chart.selection.Hide()
		if r.chart.hovered && r.chart.drag == nil && r.chart.pan == nil && r.chart.hoverGroup == nil {
			r.chart.updateHover(r.chart.hoverPosition)
		}
	}
	if r.chart.hoverGroup != nil {
		r.chart.hoverGroup.refresh()
	}
	r.plot.Refresh()
}

func (data plotData) hoverHint() string {
	hint := chartHint
	if data.AverageWindow > 0 {
		hint += " · " + data.AverageWindow.String() + " avg"
	} else if data.IndividualHits {
		if data.HitUnit != "" {
			hint += " · individual events · " + data.HitUnit
		} else {
			hint += " · individual hits · damage"
		}
	}
	if data.AverageWindow > 0 && data.RateUnit != "" {
		hint += " · " + data.RateUnit
	}
	if data.SecondaryUnit != "" {
		hint += " · " + data.SecondaryUnit + " on right axis"
	}
	return hint
}

func (chart *damageChart) renderPlot(w, h int) image.Image {
	chart.mu.RLock()
	defer chart.mu.RUnlock()
	palette := chart.palette
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{palette.background}, image.Point{}, draw.Src)
	grid := palette.grid
	for i := 0; i <= 4; i++ {
		drawLine(img, 0, i*(h-1)/4, w-1, i*(h-1)/4, grid)
		drawLine(img, i*(w-1)/4, 0, i*(w-1)/4, h-1, grid)
	}
	data := chart.data
	if data.Maximum <= 0 || !data.End.After(data.Start) {
		return img
	}
	// Stroke at subpixel coordinates so diagonals and joins have soft edges,
	// while keeping every sample at its original time and DPS value.
	scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
	stroke := rasterx.NewStroker(w, h, scanner)
	lineWidth := 1.25
	if logicalWidth := chart.Size().Width - chartPlotLeft - data.plotRight(); logicalWidth > 0 {
		lineWidth *= math.Max(1, float64(w)/float64(logicalWidth))
	}
	stroke.SetStroke(fixed.Int26_6(math.Round(lineWidth*64)), 4*64, rasterx.RoundCap, nil, rasterx.RoundGap, rasterx.Round)
	duration := data.End.Sub(data.Start).Seconds()
	for _, series := range data.Series {
		ink := palette.seriesColor(series.Color)
		width := lineWidth
		if chart.selectedSeries != nil {
			if seriesKey(series) == *chart.selectedSeries {
				width *= 2.5
			} else {
				ink = dimSeries(ink, palette.background)
			}
		}
		stroke.SetStroke(fixed.Int26_6(math.Round(width*64)), 4*64, rasterx.RoundCap, nil, rasterx.RoundGap, rasterx.Round)
		if data.IndividualHits {
			for _, hit := range series.Hits {
				if hit.Second < data.Start.Unix() || hit.Second >= data.End.Unix() {
					continue
				}
				x := float64(hit.Second-data.Start.Unix()) / duration * float64(w-1)
				y := float64(h-1) * (1 - data.seriesFraction(series, hit.Damage))
				drawHitPoint(img, x+0.5, y+0.5, 2.5*width/1.25, ink)
			}
			continue
		}
		if len(series.Values) == 0 {
			continue
		}
		stroke.Clear()
		stroke.SetColor(ink)
		var last fixed.Point26_6
		for bin, value := range series.Values {
			x := float64(bin)*data.Bucket.Seconds()/duration*float64(w-1) + 0.5
			y := float64(h-1)*(1-data.seriesFraction(series, value)) + 0.5
			last = fixed.Point26_6{X: fixed.Int26_6(math.Round(x * 64)), Y: fixed.Int26_6(math.Round(y * 64))}
			if bin == 0 {
				stroke.Start(last)
			} else {
				stroke.Line(last)
			}
		}
		// Extend the final bucket, including a single-bucket burst, to the edge.
		stroke.Line(fixed.Point26_6{X: fixed.Int26_6(w*64 - 32), Y: last.Y})
		stroke.Stop(false)
		stroke.Draw()
	}
	markerColor := palette.marker
	for _, marker := range data.Markers {
		if marker.At.Before(data.Start) || !marker.At.Before(data.End) {
			continue
		}
		x := int(math.Round(float64(marker.At.Sub(data.Start)) / float64(data.End.Sub(data.Start)) * float64(w-1)))
		drawLine(img, x, 0, x, h-1, markerColor)
	}
	return img
}

func drawHitPoint(img *image.RGBA, x, y, radius float64, ink color.NRGBA) {
	for row := max(0, int(math.Floor(y-radius-1))); row <= min(img.Bounds().Max.Y-1, int(math.Ceil(y+radius+1))); row++ {
		for column := max(0, int(math.Floor(x-radius-1))); column <= min(img.Bounds().Max.X-1, int(math.Ceil(x+radius+1))); column++ {
			coverage := math.Max(0, math.Min(1, radius+0.5-math.Hypot(float64(column)+0.5-x, float64(row)+0.5-y)))
			if coverage == 0 {
				continue
			}
			alpha := coverage * float64(ink.A) / 255
			old := img.RGBAAt(column, row)
			img.SetRGBA(column, row, color.RGBA{
				R: uint8(math.Round(float64(ink.R)*alpha + float64(old.R)*(1-alpha))),
				G: uint8(math.Round(float64(ink.G)*alpha + float64(old.G)*(1-alpha))),
				B: uint8(math.Round(float64(ink.B)*alpha + float64(old.B)*(1-alpha))), A: 255,
			})
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, ink color.NRGBA) {
	dx, dy := int(math.Abs(float64(x1-x0))), -int(math.Abs(float64(y1-y0)))
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		img.Set(x0, y0, ink)
		if x0 == x1 && y0 == y1 {
			return
		}
		e := 2 * err
		if e >= dy {
			err += dy
			x0 += sx
		}
		if e <= dx {
			err += dx
			y0 += sy
		}
	}
}

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

func (chart *damageChart) beginRangeDrag(position fyne.Position) bool {
	width := chart.Size().Width - chartPlotLeft - chart.plotRight()
	if width <= 0 || position.X < chartPlotLeft || position.X > chartPlotLeft+width || position.Y < chartPlotTop || position.Y > chart.Size().Height-chartPlotBottom {
		return false
	}
	chart.mu.RLock()
	start, end := chart.data.Start, chart.data.End
	chart.mu.RUnlock()
	if !end.After(start) {
		return false
	}
	x := position.X - chartPlotLeft
	chart.drag = &chartRangeDrag{Start: start, End: end, Width: width, Anchor: x, Current: x}
	if chart.hoverGroup != nil {
		chart.hoverGroup.leave(chart)
	}
	return true
}

func (chart *damageChart) MouseDown(event *desktop.MouseEvent) {
	chart.pan = nil
	chart.cancelRangeDrag()
	if event.Button == desktop.MouseButtonSecondary {
		chart.dragBlocked = true
		chart.beginPan(event.Position)
		return
	}
	chart.dragBlocked = event.Button != desktop.MouseButtonPrimary
	if !chart.dragBlocked {
		chart.dragBlocked = !chart.beginRangeDrag(event.Position)
		if !chart.dragBlocked {
			if canvas := fyne.CurrentApp().Driver().CanvasForObject(chart); canvas != nil {
				canvas.Unfocus()
			}
		}
	}
}

func (chart *damageChart) MouseUp(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonSecondary {
		chart.updatePan(event.Position)
		chart.pan = nil
	}
	if event.Button == desktop.MouseButtonPrimary {
		click := chart.drag != nil && !chart.drag.Moved && !chart.dragBlocked
		// Some drivers send MouseUp before DragEnd; GLFW sends it afterwards.
		// Finishing clears the gesture, so either order applies the range once.
		if chart.drag != nil && chart.drag.Moved {
			chart.drag.Current = max(float32(0), min(chart.drag.Width, event.Position.X-chartPlotLeft))
			chart.drag.Moved = math.Abs(float64(chart.drag.Current-chart.drag.Anchor)) >= float64(chartDragThreshold)
		}
		chart.DragEnd()
		if click {
			chart.selectSeriesAt(event.Position)
		}
	}
	chart.dragBlocked = false
}

func (chart *damageChart) Dragged(event *fyne.DragEvent) {
	if chart.pan != nil {
		chart.updatePan(event.Position)
		return
	}
	if chart.dragBlocked {
		return
	}
	if chart.drag == nil {
		// Touch and programmatic drag events do not have a preceding MouseDown.
		origin := fyne.NewPos(event.Position.X-event.Dragged.DX, event.Position.Y-event.Dragged.DY)
		if !chart.beginRangeDrag(origin) {
			chart.dragBlocked = true
			return
		}
	}
	chart.drag.Current = max(float32(0), min(chart.drag.Width, event.Position.X-chartPlotLeft))
	chart.drag.Moved = math.Abs(float64(chart.drag.Current-chart.drag.Anchor)) >= float64(chartDragThreshold)
	chart.updateRangeSelection()
}

func (drag *chartRangeDrag) selectedRange() (time.Time, time.Time) {
	left, right := min(drag.Anchor, drag.Current), max(drag.Anchor, drag.Current)
	duration := float64(drag.End.Sub(drag.Start))
	start := drag.Start.Add(time.Duration(float64(left) / float64(drag.Width) * duration)).Truncate(time.Second)
	rawEnd := drag.Start.Add(time.Duration(float64(right) / float64(drag.Width) * duration))
	end := rawEnd.Truncate(time.Second)
	if end.Before(rawEnd) {
		end = end.Add(time.Second)
	}
	if start.Before(drag.Start) {
		start = drag.Start
	}
	if end.After(drag.End) {
		end = drag.End
	}
	return start, end
}

func selectionDuration(duration time.Duration) string {
	unit := func(count int64, name string) string {
		if count != 1 {
			name += "s"
		}
		return fmt.Sprintf("%d %s", count, name)
	}
	hours, minutes := int64(duration/time.Hour), int64(duration/time.Minute)%60
	if hours > 0 {
		label := unit(hours, "hour")
		if minutes > 0 {
			label += " " + unit(minutes, "minute")
		}
		return label
	}
	if minutes > 0 {
		return unit(minutes, "minute")
	}
	return unit(int64(duration/time.Second), "second")
}

func (chart *damageChart) updateRangeSelection() {
	if chart.selection == nil || chart.drag == nil {
		return
	}
	drag := chart.drag
	chart.cursor.Hide()
	if !drag.Moved {
		chart.selection.Hide()
		return
	}
	left, right := min(drag.Anchor, drag.Current), max(drag.Anchor, drag.Current)
	chart.selection.Move(fyne.NewPos(chartPlotLeft+left, chartPlotTop))
	chart.selection.Resize(fyne.NewSize(right-left, max(float32(1), chart.Size().Height-chartPlotTop-chartPlotBottom)))
	chart.selection.Show()
	start, end := drag.selectedRange()
	chart.setHoverHint("Select " + start.UTC().Format("Jan 02 15:04:05") + " – " + end.UTC().Format("Jan 02 15:04:05") + " EVE time · (" + selectionDuration(end.Sub(start)) + ") · release to zoom")
}

func (chart *damageChart) cancelRangeDrag() {
	chart.drag = nil
	if chart.selection != nil {
		chart.selection.Hide()
	}
	chart.setHoverHint(chartHint)
}

func (chart *damageChart) DragEnd() {
	if chart.pan != nil {
		chart.pan = nil
		chart.dragBlocked = false
		return
	}
	drag := chart.drag
	chart.cancelRangeDrag()
	chart.dragBlocked = false
	if drag == nil || !drag.Moved || chart.OnRangeSelected == nil {
		return
	}
	start, end := drag.selectedRange()
	if end.After(start) {
		chart.OnRangeSelected(start, end)
	}
}
