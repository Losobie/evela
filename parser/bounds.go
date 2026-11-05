package parser

import "time"

// ActivityBounds includes supported activities and structure markers, including
// zero-damage misses. Chat and travel alone do not set graph activity bounds.
// Parsed slices are ordered by time, so damage points need only their endpoints.
func (log *Log) ActivityBounds() (first, last time.Time) {
	include := func(second int64) {
		at := time.Unix(second, 0).UTC()
		if first.IsZero() || at.Before(first) {
			first = at
		}
		if last.IsZero() || at.After(last) {
			last = at
		}
	}
	if len(log.Points) > 0 {
		include(log.Points[0].Second)
		include(log.Points[len(log.Points)-1].Second)
	}
	for _, hit := range log.Hits {
		include(hit.Second)
	}
	for _, event := range log.StructureEvents {
		include(event.Second)
	}
	for _, event := range log.EWar {
		include(event.Second)
	}
	for _, event := range log.Logistics {
		include(event.Second)
	}
	for _, event := range log.Mining {
		include(event.Second)
	}
	return first, last
}
