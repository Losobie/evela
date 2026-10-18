package main

import "time"

const dpsAverageWindow = 10 * time.Second

// Average each second's damage over that second and the preceding nine seconds,
// including idle seconds. Then average those rolling DPS samples into the visible
// buckets. This stays accurate with coarse buckets and partial final buckets.
func (view *damageView) tenSecondAverage(snapshot *logSnapshot) []characterSeries {
	if view.averageSeries != nil {
		return view.averageSeries
	}
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
	window := int64(dpsAverageWindow / time.Second)
	for _, log := range snapshot.Logs {
		index, ok := byCharacter[log.Character]
		if !ok {
			continue
		}
		series := &view.averageSeries[index]
		for _, point := range log.Points {
			// Each hit contributes damage/10 DPS to its own second and the next
			// nine. Include pre-range hits whose contribution reaches the range.
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
