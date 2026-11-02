package main

import (
	"math"
	"sort"
	"time"
)

func (view damageView) damageAttributeSeries(snapshot *logSnapshot, selected map[string]bool, mode string, averageWindow time.Duration) []characterSeries {
	if view.Bucket < time.Second || !view.End.After(view.Start) {
		return nil
	}
	active := map[string]bool{}
	for _, series := range view.activeSeries() {
		if selected == nil || selected[series.Name] {
			active[series.Name] = true
		}
	}
	if len(active) == 0 {
		return nil
	}
	type groupData struct {
		series  characterSeries
		members map[string]bool
		bins    map[int]map[string]bool
		hulls   map[string]map[string]bool
	}
	groups := map[string]*groupData{}
	first, end := view.Start.Unix(), view.End.Unix()
	bucket := int64(view.Bucket / time.Second)
	n := int(math.Ceil(view.End.Sub(view.Start).Seconds() / view.Bucket.Seconds()))
	window := int64(1)
	if averageWindow >= time.Second {
		window = int64(averageWindow / time.Second)
	}
	for _, log := range snapshot.Logs {
		if !active[log.Character] {
			continue
		}
		for _, hit := range log.Hits {
			if targetGroupingMode(mode) && hit.Incoming {
				continue
			}
			point := damagePoint{Second: hit.Second}
			if hit.Incoming {
				point.Incoming = hit.Amount
				if hit.WeaponStrength != "Misses" {
					point.IncomingHits = 1
				}
			} else {
				point.Outgoing = hit.Amount
				if hit.WeaponStrength != "Misses" {
					point.OutgoingHits = 1
				}
			}
			left, right := max(first, point.Second), min(end, point.Second+window)
			if left >= right {
				continue
			}
			name := hitDamageAttributeOption(hit, mode).Label
			hullName := name
			group := groups[name]
			if group == nil {
				group = &groupData{
					series: characterSeries{
						Name: name, Incoming: make([]float64, n), Outgoing: make([]float64, n),
						MemberCounts: make([]int, n), MemberShips: map[string][]string{},
					},
					members: map[string]bool{}, bins: map[int]map[string]bool{}, hulls: map[string]map[string]bool{},
				}
				groups[name] = group
			}
			group.members[log.Character] = true
			if group.hulls[log.Character] == nil {
				group.hulls[log.Character] = map[string]bool{}
			}
			group.hulls[log.Character][hullName] = true
			if point.Second >= first && point.Second < end {
				group.series.TotalIncoming += point.Incoming
				group.series.TotalOutgoing += point.Outgoing
				group.series.IncomingHits += point.IncomingHits
				group.series.OutgoingHits += point.OutgoingHits
			}
			for bin := int((left - first) / bucket); bin <= int((right-1-first)/bucket); bin++ {
				binStart := first + int64(bin)*bucket
				binEnd := min(end, binStart+bucket)
				overlap := min(right, binEnd) - max(left, binStart)
				weight := float64(overlap) / float64(window) / float64(binEnd-binStart)
				group.series.Incoming[bin] += point.Incoming * weight
				group.series.Outgoing[bin] += point.Outgoing * weight
				if group.bins[bin] == nil {
					group.bins[bin] = map[string]bool{}
				}
				group.bins[bin][log.Character] = true
			}
		}
	}
	var result []characterSeries
	for _, group := range groups {
		for name := range group.members {
			group.series.Members = append(group.series.Members, name)
			for hull := range group.hulls[name] {
				group.series.MemberShips[name] = append(group.series.MemberShips[name], hull)
			}
			sort.Strings(group.series.MemberShips[name])
		}
		sort.Strings(group.series.Members)
		for bin, members := range group.bins {
			group.series.MemberCounts[bin] = len(members)
		}
		result = append(result, group.series)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
