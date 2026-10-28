package main

import "time"

const dpsAverageWindow = 10 * time.Second

// Average damage over the requested window, including idle seconds, then average
// those rolling rates into the visible buckets, including partial final buckets.
func (view *damageView) rollingAverage(snapshot *logSnapshot, averageWindow time.Duration) []characterSeries {
	if averageWindow < time.Second {
		return view.activeSeries()
	}
	if view.averageSeries != nil && view.averageWindow == averageWindow {
		return view.averageSeries
	}
	view.averageWindow = averageWindow
	active := view.activeSeries()
	view.averageSeries = make([]characterSeries, len(active))
	byCharacter := make(map[string]int, len(active))
	for index, series := range active {
		byCharacter[series.Name] = index
		view.averageSeries[index] = series
		view.averageSeries[index].Incoming = make([]float64, len(series.Incoming))
		view.averageSeries[index].Outgoing = make([]float64, len(series.Outgoing))
	}
	if len(active) == 0 || view.Bucket < time.Second || !view.End.After(view.Start) {
		return view.averageSeries
	}
	first, end := view.Start.Unix(), view.End.Unix()
	bucket := int64(view.Bucket / time.Second)
	window := int64(averageWindow / time.Second)
	for _, log := range snapshot.Logs {
		index, ok := byCharacter[log.Character]
		if !ok {
			continue
		}
		series := &view.averageSeries[index]
		for _, point := range log.Points {
			// Include pre-range hits whose contribution reaches the visible range.
			left, right := max(first, point.Second), min(end, point.Second+window)
			if left >= right {
				continue
			}
			firstBin, lastBin := (left-first)/bucket, (right-1-first)/bucket
			for bin := firstBin; bin <= lastBin; bin++ {
				binStart := first + bin*bucket
				binEnd := min(end, binStart+bucket)
				overlap := min(right, binEnd) - max(left, binStart)
				weight := float64(overlap) / float64(window) / float64(binEnd-binStart)
				series.Incoming[bin] += point.Incoming * weight
				series.Outgoing[bin] += point.Outgoing * weight
			}
		}
	}
	return view.averageSeries
}
