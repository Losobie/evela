package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

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
