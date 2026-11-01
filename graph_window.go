package main

import (
	"sort"
	"time"
)

// Parsed records are sorted once during import. Zooming can then share slices of
// the archive instead of scanning or copying records outside the visible range.
func recordsInRange[T any](records []T, first, end int64, second func(T) int64) []T {
	left := sort.Search(len(records), func(i int) bool { return second(records[i]) >= first })
	right := left + sort.Search(len(records)-left, func(i int) bool { return second(records[left+i]) >= end })
	return records[left:right:right]
}

func snapshotInRange(snapshot *logSnapshot, start, end time.Time, lookback time.Duration) *logSnapshot {
	result := *snapshot
	result.Logs = nil
	first, last := start.Add(-lookback).Unix(), end.Unix()
	for _, log := range snapshot.Logs {
		copyLog := *log
		copyLog.Points = recordsInRange(log.Points, first, last, func(point damagePoint) int64 { return point.Second })
		copyLog.Hits = recordsInRange(log.Hits, first, last, func(hit damageHit) int64 { return hit.Second })
		copyLog.IncomingSources = recordsInRange(log.IncomingSources, first, last, func(point incomingSourcePoint) int64 { return point.Second })
		copyLog.StructureEvents = recordsInRange(log.StructureEvents, first, last, func(event structureEvent) int64 { return event.Second })
		copyLog.EWar = recordsInRange(log.EWar, first, last, func(event ewarEvent) int64 { return event.Second })
		copyLog.Logistics = recordsInRange(log.Logistics, first, last, func(event logisticsEvent) int64 { return event.Second })
		copyLog.Mining = recordsInRange(log.Mining, first, last, func(event miningEvent) int64 { return event.Second })
		copyLog.Events = recordsInRange(log.Events, first, last, func(event logEvent) int64 { return event.Second })
		if len(copyLog.Points)+len(copyLog.Hits)+len(copyLog.IncomingSources)+len(copyLog.StructureEvents)+len(copyLog.EWar)+len(copyLog.Logistics)+len(copyLog.Mining)+len(copyLog.Events) == 0 {
			continue
		}
		result.Logs = append(result.Logs, &copyLog)
	}
	return &result
}
