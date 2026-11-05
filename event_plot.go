package main

import (
	"math"
	"sort"
	"time"
)

// Logistics, E-War and mining differ in filtering, grouping and units. Their
// event-to-rate calculation is shared, including lookback and partial buckets.
type eventPlotBuilder struct {
	first, end, bucket, window int64
	buckets                    int
	groups                     map[chartSeriesKey]*plotSeries
}

func newEventPlotBuilder(data *plotData, average time.Duration) (eventPlotBuilder, bool) {
	data.IndividualHits = average == 0
	window := int64(1)
	if average > 0 {
		data.AverageWindow, window = average, int64(average/time.Second)
	}
	if !data.End.After(data.Start) || data.Bucket < time.Second {
		return eventPlotBuilder{}, false
	}
	return eventPlotBuilder{first: data.Start.Unix(), end: data.End.Unix(), bucket: int64(data.Bucket / time.Second), window: window,
		buckets: int(math.Ceil(data.End.Sub(data.Start).Seconds() / data.Bucket.Seconds()))}, true
}

func (builder *eventPlotBuilder) includes(second int64) bool {
	return max(builder.first, second) < min(builder.end, second+builder.window)
}

func (builder *eventPlotBuilder) add(name, unit string, hit plotHit) {
	left, right := max(builder.first, hit.Second), min(builder.end, hit.Second+builder.window)
	if left >= right {
		return
	}
	key := chartSeriesKey{name, unit}
	series := builder.groups[key]
	if series == nil {
		if builder.groups == nil {
			builder.groups = make(map[chartSeriesKey]*plotSeries)
		}
		series = &plotSeries{Name: name, Unit: unit, Color: characterColor(name), Values: make([]float64, builder.buckets)}
		builder.groups[key] = series
	}
	if hit.Second >= builder.first {
		series.Hits = append(series.Hits, hit)
	}
	for bin := int((left - builder.first) / builder.bucket); bin <= int((right-1-builder.first)/builder.bucket); bin++ {
		start, end := builder.first+int64(bin)*builder.bucket, min(builder.end, builder.first+int64(bin+1)*builder.bucket)
		overlap := min(right, end) - max(left, start)
		series.Values[bin] += hit.Damage * float64(overlap) / float64(builder.window) / float64(end-start)
	}
}

func (builder *eventPlotBuilder) series() []plotSeries {
	var result []plotSeries
	for _, series := range builder.groups {
		sort.SliceStable(series.Hits, func(i, j int) bool { return series.Hits[i].Second < series.Hits[j].Second })
		result = append(result, *series)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
