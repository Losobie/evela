package main

import (
	"image/color"
	"math"
	"time"
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
type chartSeriesKey struct{ name, unit string }

func seriesKey(series plotSeries) chartSeriesKey {
	return chartSeriesKey{series.Name, series.Unit}
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
