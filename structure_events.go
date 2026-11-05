package main

import (
	"sort"
	"time"
)

type plotMarker struct {
	At    time.Time
	Label string
}

func (view damageView) structureMarkers(snapshot *logSnapshot, selected map[string]bool) []plotMarker {
	seen := map[structureEvent]bool{}
	var markers []plotMarker
	for _, log := range snapshot.Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.StructureEvents {
			at := time.Unix(event.Second, 0).UTC()
			if at.Before(view.Start) || !at.Before(view.End) || seen[event] {
				continue
			}
			seen[event] = true
			markers = append(markers, plotMarker{At: at, Label: event.Target})
		}
	}
	sort.Slice(markers, func(i, j int) bool {
		if markers[i].At.Equal(markers[j].At) {
			return markers[i].Label < markers[j].Label
		}
		return markers[i].At.Before(markers[j].At)
	})
	return markers
}
