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
	Color        color.NRGBA
	Values       []float64
	MemberCount  int
	MemberCounts []int
}
type plotData struct {
	Start, End    time.Time
	Bucket        time.Duration
	Series        []plotSeries
	Maximum       float64
	Logarithmic   bool
	AverageWindow time.Duration
}

// Shift by one before taking the log so zero-DPS buckets retain their baseline.
// The inverse is used for tick labels, keeping the axis and hover values in DPS.
func (data plotData) fraction(value float64) float64 {
	if data.Maximum <= 0 {
		return 0
	}
	value = math.Max(0, math.Min(data.Maximum, value))
	if data.Logarithmic {
		return math.Log1p(value) / math.Log1p(data.Maximum)
	}
	return value / data.Maximum
}

func (data plotData) valueAt(fraction float64) float64 {
	fraction = math.Max(0, math.Min(1, fraction))
	if data.Logarithmic {
		return math.Expm1(math.Log1p(data.Maximum) * fraction)
	}
	return data.Maximum * fraction
}

type damageChart struct {
	widget.BaseWidget
	mu              sync.RWMutex
	data            plotData
	hover           *canvas.Text
	hoverReadings   [3]*canvas.Text
	hoverClip       *container.Scroll
	hoverPosition   fyne.Position
	hovered         bool
	cursor          *canvas.Line
	selection       *canvas.Rectangle
	drag            *chartRangeDrag
	dragBlocked     bool
	OnRangeSelected func(time.Time, time.Time)
	OnWheelZoom     func(time.Time, time.Time)
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
	chartHint                  = "Wheel or drag to zoom · Undo: Ctrl+Z / Back · Redo: Ctrl+Y / Forward · UTC"
)

var _ fyne.Draggable = (*damageChart)(nil)
var _ desktop.Mouseable = (*damageChart)(nil)

func newDamageChart() *damageChart {
	chart := &damageChart{}
	chart.ExtendBaseWidget(chart)
	return chart
}

func (chart *damageChart) setData(data plotData) {
	// A refresh with the same time axis may update the data during a drag.
	// A changed axis cancels it so the gesture cannot select the wrong times.
	if chart.drag != nil && (!chart.drag.Start.Equal(data.Start) || !chart.drag.End.Equal(data.End)) {
		chart.cancelRangeDrag()
		chart.dragBlocked = true
	}
	for _, series := range data.Series {
		for _, value := range series.Values {
			if value > data.Maximum {
				data.Maximum = value
			}
		}
	}
	if data.Maximum <= 0 {
		data.Maximum = 1
	} else {
		data.Maximum *= 1.08
	}
	chart.mu.Lock()
	chart.data = data
	chart.mu.Unlock()
	chart.Refresh()
}

func (chart *damageChart) CreateRenderer() fyne.WidgetRenderer {
	muted := color.NRGBA{150, 166, 187, 255}
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
	// or allowing text to spill across its pan buttons.
	chart.hoverClip = container.NewHScroll(container.NewHBox(hoverObjects...))
	chart.hoverClip.Direction = container.ScrollNone
	chart.cursor = canvas.NewLine(color.NRGBA{170, 185, 200, 180})
	chart.cursor.Hide()
	chart.selection = canvas.NewRectangle(color.NRGBA{68, 209, 223, 55})
	chart.selection.StrokeColor = color.NRGBA{68, 209, 223, 220}
	chart.selection.StrokeWidth = 1
	chart.selection.Hide()
	r := &chartRenderer{chart: chart, plot: canvas.NewRaster(chart.renderPlot)}
	r.empty = canvas.NewText("No damage in this range for the selected characters", muted)
	r.empty.TextSize = 13
	r.objects = []fyne.CanvasObject{r.plot, chart.selection, chart.cursor, chart.hoverClip, r.empty}
	for i := 0; i < 5; i++ {
		y, x := canvas.NewText("", muted), canvas.NewText("", muted)
		y.TextSize, x.TextSize = 11, 11
		r.yLabels = append(r.yLabels, y)
		r.xLabels = append(r.xLabels, x)
		r.objects = append(r.objects, y, x)
	}
	r.Refresh()
	return r
}

type chartRenderer struct {
	chart            *damageChart
	plot             *canvas.Raster
	empty            *canvas.Text
	yLabels, xLabels []*canvas.Text
	objects          []fyne.CanvasObject
}

func (r *chartRenderer) Layout(size fyne.Size) {
	w, h := fyne.Max(size.Width-chartPlotLeft-chartPlotRight, 1), fyne.Max(size.Height-chartPlotTop-chartPlotBottom, 1)
	if r.chart.drag != nil && r.chart.drag.Width != w {
		r.chart.cancelRangeDrag()
		r.chart.dragBlocked = true
	}
	r.plot.Move(fyne.NewPos(chartPlotLeft, chartPlotTop))
	r.plot.Resize(fyne.NewSize(w, h))
	r.chart.hoverClip.Move(fyne.NewPos(chartPlotLeft, 0))
	r.chart.hoverClip.Resize(fyne.NewSize(w, chartPlotTop))
	r.empty.Move(fyne.NewPos(80, 32))
	for i := 0; i < 5; i++ {
		y := float32(i)*h/4 + 19
		r.yLabels[i].Move(fyne.NewPos(2, y))
		x := chartPlotLeft + float32(i)*w/4
		if i == 4 {
			x -= r.xLabels[i].MinSize().Width
		} else if i > 0 {
			x -= r.xLabels[i].MinSize().Width / 2
		}
		r.xLabels[i].Move(fyne.NewPos(x, h+28))
	}
}
func (r *chartRenderer) MinSize() fyne.Size           { return fyne.NewSize(550, 175) }
func (r *chartRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chartRenderer) Destroy()                     {}
func (r *chartRenderer) Refresh() {
	r.chart.mu.RLock()
	data := r.chart.data
	r.chart.mu.RUnlock()
	hasDamage := false
	for _, series := range data.Series {
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
	for i := 0; i < 5; i++ {
		value := data.valueAt(float64(4-i) / 4)
		r.yLabels[i].Text = shortNumber(value)
		if value > 0 && value < 1 {
			r.yLabels[i].Text = fmt.Sprintf("%.2g", value)
		}
		r.yLabels[i].Refresh()
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
	hint := chartHint
	if data.AverageWindow > 0 {
		hint += " · " + data.AverageWindow.String() + " avg"
	}
	r.chart.setHoverHint(hint)
	if r.chart.drag != nil && r.chart.drag.Moved {
		r.chart.updateRangeSelection()
	} else {
		r.chart.selection.Hide()
		if r.chart.hovered && r.chart.drag == nil {
			r.chart.updateHover(r.chart.hoverPosition)
		}
	}
	r.plot.Refresh()
}

func (chart *damageChart) renderPlot(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.NRGBA{17, 23, 33, 255}}, image.Point{}, draw.Src)
	grid := color.NRGBA{42, 53, 69, 255}
	for i := 0; i <= 4; i++ {
		drawLine(img, 0, i*(h-1)/4, w-1, i*(h-1)/4, grid)
		drawLine(img, i*(w-1)/4, 0, i*(w-1)/4, h-1, grid)
	}
	chart.mu.RLock()
	defer chart.mu.RUnlock()
	data := chart.data
	if data.Maximum <= 0 || !data.End.After(data.Start) {
		return img
	}
	for _, series := range data.Series {
		lastX, lastY := 0, h-1
		for bin, value := range series.Values {
			x := int(float64(bin) * data.Bucket.Seconds() / data.End.Sub(data.Start).Seconds() * float64(w-1))
			y := h - 1 - int(data.fraction(value)*float64(h-1))
			if bin > 0 {
				drawLine(img, lastX, lastY, x, y, series.Color)
			}
			// Keep one-bucket and very short bursts visible.
			drawLine(img, x, y, x, y-1, series.Color)
			lastX, lastY = x, y
		}
		if len(series.Values) > 0 {
			drawLine(img, lastX, lastY, w-1, lastY, series.Color)
		}
	}
	return img
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
	chart.hovered = false
	if chart.cursor != nil {
		chart.cursor.Hide()
	}
}
func (chart *damageChart) MouseMoved(event *desktop.MouseEvent) {
	chart.hoverPosition, chart.hovered = event.Position, true
	if chart.drag != nil {
		return
	}
	chart.updateHover(event.Position)
}

// Recompute from the pointer position so refreshes inspect the new data and
// time axis even when the mouse has not moved.
func (chart *damageChart) updateHover(position fyne.Position) {
	chart.mu.RLock()
	data := chart.data
	chart.mu.RUnlock()
	if len(data.Series) == 0 || data.Bucket <= 0 || !data.End.After(data.Start) || chart.hover == nil {
		return
	}
	w := chart.Size().Width - chartPlotLeft - chartPlotRight
	if w <= 0 {
		return
	}
	x := math.Max(0, math.Min(float64(w), float64(position.X-chartPlotLeft)))
	elapsed := x / float64(w) * data.End.Sub(data.Start).Seconds()
	bin := int(elapsed / data.Bucket.Seconds())
	bin = min(bin, int((data.End.Sub(data.Start)-1)/data.Bucket))
	type reading struct {
		name  string
		value float64
		color color.NRGBA
	}
	var readings []reading
	for _, series := range data.Series {
		if bin >= 0 && bin < len(series.Values) {
			name := series.Name
			count := series.MemberCount
			if bin < len(series.MemberCounts) {
				count = series.MemberCounts[bin]
			}
			if count > 1 {
				name = fmt.Sprintf("%s (%d)", name, count)
			}
			readings = append(readings, reading{name, series.Values[bin], series.Color})
		}
	}
	sort.SliceStable(readings, func(i, j int) bool { return readings[i].value > readings[j].value })
	chart.hover.Text = data.Start.Add(time.Duration(bin)*data.Bucket).Format("Jan 02 15:04:05") + " UTC"
	for _, label := range chart.hoverReadings {
		label.Hide()
	}
	for i, reading := range readings {
		if i == 3 {
			break
		}
		rate, unit := shortNumber(reading.value), "DPS"
		if data.AverageWindow > 0 {
			if reading.value < 1000 {
				rate = fmt.Sprintf("%.1f", reading.value)
			}
			unit += " (" + data.AverageWindow.String() + " avg)"
		}
		label := chart.hoverReadings[i]
		label.Text = fmt.Sprintf("· %s: %s %s", reading.name, rate, unit)
		label.Color = reading.color
		label.Show()
	}
	chart.hoverClip.Refresh()
	chart.cursor.Position1 = fyne.NewPos(float32(x)+chartPlotLeft, chartPlotTop)
	chart.cursor.Position2 = fyne.NewPos(float32(x)+chartPlotLeft, chart.Size().Height-chartPlotBottom)
	chart.cursor.Show()
	chart.cursor.Refresh()
}

func (chart *damageChart) setHoverHint(text string) {
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
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
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
	return true
}

func (chart *damageChart) MouseDown(event *desktop.MouseEvent) {
	chart.cancelRangeDrag()
	chart.dragBlocked = event.Button != desktop.MouseButtonPrimary
	if !chart.dragBlocked {
		chart.dragBlocked = !chart.beginRangeDrag(event.Position)
	}
}

func (chart *damageChart) MouseUp(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary {
		// Some drivers send MouseUp before DragEnd; GLFW sends it afterwards.
		// Finishing clears the gesture, so either order applies the range once.
		if chart.drag != nil && chart.drag.Moved {
			chart.drag.Current = max(float32(0), min(chart.drag.Width, event.Position.X-chartPlotLeft))
			chart.drag.Moved = math.Abs(float64(chart.drag.Current-chart.drag.Anchor)) >= float64(chartDragThreshold)
		}
		chart.DragEnd()
	}
	chart.dragBlocked = false
}

func (chart *damageChart) Dragged(event *fyne.DragEvent) {
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
	chart.setHoverHint("Select " + start.UTC().Format("Jan 02 15:04:05") + " – " + end.UTC().Format("Jan 02 15:04:05") + " UTC · release to zoom")
}

func (chart *damageChart) cancelRangeDrag() {
	chart.drag = nil
	if chart.selection != nil {
		chart.selection.Hide()
	}
	chart.setHoverHint(chartHint)
}

func (chart *damageChart) DragEnd() {
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
