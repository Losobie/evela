package main

import (
	"fmt"
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

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
