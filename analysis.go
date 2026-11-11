package main

import (
	"fmt"
	"math"
	"slices"
	"time"
)

type characterSeries struct {
	IncomingMisses, OutgoingMisses bool
	Name                           string
	Mining                         *miningAmounts
	Members                        []string
	MemberCounts                   []int
	MemberShips                    map[string][]string
	Incoming, Outgoing             []float64
	TotalIncoming, TotalOutgoing   float64
	IncomingHits, OutgoingHits     int
}

type damageView struct {
	Start, End    time.Time
	Bucket        time.Duration
	Series        []characterSeries
	averageSeries []characterSeries
	averageWindow time.Duration
}

// Keep the archive roster in Series, but expose only characters with actual
// incoming/outgoing records in this window to filters, plots and totals.
func (view damageView) activeSeries() []characterSeries {
	var active []characterSeries
	for _, series := range view.Series {
		if series.IncomingMisses || series.OutgoingMisses || series.TotalIncoming > 0 || series.TotalOutgoing > 0 || series.IncomingHits > 0 || series.OutgoingHits > 0 {
			active = append(active, series)
		}
	}
	return active
}

func (series characterSeries) hasActivity(incoming bool) bool {
	total, hits, misses, values := series.TotalOutgoing, series.OutgoingHits, series.OutgoingMisses, series.Outgoing
	if incoming {
		total, hits, misses, values = series.TotalIncoming, series.IncomingHits, series.IncomingMisses, series.Incoming
	}
	// A pre-range hit may still contribute to a visible rolling average.
	return total > 0 || hits > 0 || misses || slices.ContainsFunc(values, func(value float64) bool { return value > 0 })
}

func buildView(snapshot *logSnapshot, start, end time.Time, requestedBucket time.Duration) damageView {
	view := damageView{Start: start, End: end, Bucket: requestedBucket}
	seconds := end.Sub(start).Seconds()
	if seconds <= 0 {
		return view
	}
	// Bound rendering/memory for broad date ranges. Zero requests Auto.
	minBucket := time.Duration(math.Ceil(seconds/1200)) * time.Second
	if view.Bucket < minBucket {
		view.Bucket = minBucket
	}
	if view.Bucket < time.Second {
		view.Bucket = time.Second
	}
	n := int(math.Ceil(seconds / view.Bucket.Seconds()))
	byCharacter := make(map[string]int)
	for _, name := range snapshot.Characters {
		byCharacter[name] = len(view.Series)
		view.Series = append(view.Series, characterSeries{Name: name, Incoming: make([]float64, n), Outgoing: make([]float64, n)})
	}
	for _, log := range snapshot.Logs {
		series := &view.Series[byCharacter[log.Character]]
		for _, hit := range log.Hits {
			if hit.WeaponStrength == "Misses" && hit.Second >= start.Unix() && hit.Second < end.Unix() {
				if hit.Incoming {
					series.IncomingMisses = true
				} else {
					series.OutgoingMisses = true
				}
			}
		}
		for _, point := range log.Points {
			if point.Second < start.Unix() || point.Second >= end.Unix() {
				continue
			}
			bin := int((point.Second - start.Unix()) / int64(view.Bucket/time.Second))
			series.Incoming[bin] += point.Incoming
			series.Outgoing[bin] += point.Outgoing
			series.TotalIncoming += point.Incoming
			series.TotalOutgoing += point.Outgoing
			series.IncomingHits += point.IncomingHits
			series.OutgoingHits += point.OutgoingHits
		}
	}
	for s := range view.Series {
		for bin := 0; bin < n; bin++ {
			// The last partial bucket uses its actual duration.
			width := math.Min(view.Bucket.Seconds(), seconds-float64(bin)*view.Bucket.Seconds())
			view.Series[s].Incoming[bin] /= width
			view.Series[s].Outgoing[bin] /= width
		}
	}
	return view
}

func presetRange(name string, snapshot *logSnapshot) (time.Time, time.Time) {
	latest := snapshot.Last
	if latest.IsZero() {
		latest = time.Now().UTC()
	}
	end := latest.Truncate(time.Second).Add(time.Second)
	switch name {
	case "Last 10 minutes":
		return end.Add(-10 * time.Minute), end
	case "Last 30 minutes":
		return end.Add(-30 * time.Minute), end
	case "Last hour":
		return end.Add(-time.Hour), end
	case "Last 24 hours":
		return end.Add(-24 * time.Hour), end
	case "Last 7 days":
		return end.Add(-7 * 24 * time.Hour), end
	case "All history":
		if !snapshot.First.IsZero() {
			return snapshot.First, end
		}
	}
	return end.Add(-30 * time.Minute), end
}

func parseRangeTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if at, err := time.Parse(layout, value); err == nil {
			return at, nil
		}
	}
	return time.Time{}, fmt.Errorf("use YYYY-MM-DD or YYYY-MM-DD HH:MM[:SS] (EVE time)")
}

func parseRange(startText, endText string) (time.Time, time.Time, error) {
	start, err := parseRangeTime(startText)
	if err != nil {
		return start, time.Time{}, err
	}
	end, err := parseRangeTime(endText)
	if err != nil {
		return start, end, err
	}
	if !end.After(start) {
		return start, end, fmt.Errorf("end must be later than start (end is exclusive)")
	}
	return start, end, nil
}

func shortNumber(value float64) string {
	switch {
	case value >= 1e9:
		return fmt.Sprintf("%.2fB", value/1e9)
	case value >= 1e6:
		return fmt.Sprintf("%.2fM", value/1e6)
	case value >= 1e3:
		return fmt.Sprintf("%.1fk", value/1e3)
	default:
		return fmt.Sprintf("%.0f", value)
	}
}
