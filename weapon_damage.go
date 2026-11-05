package main

import (
	"sort"
	"strings"
)

const (
	filterWeaponType     = "Weapon Type"
	filterWeaponStrength = "Weapon Strength"
)

func damageAttributeGroupingMode(mode string) bool {
	return mode == filterWeaponType || mode == filterWeaponStrength || targetGroupingMode(mode)
}
func namedDamageOption(value, kind string) damageFilterOption {
	if value == "" {
		return damageFilterOption{Key: "unknown", Label: "Unknown " + strings.ToLower(kind)}
	}
	return damageFilterOption{Key: "name:" + value, Label: value}
}
func hitDamageAttributeOption(hit damageHit, mode string) damageFilterOption {
	if targetGroupingMode(mode) {
		return namedDamageOption(targetValue(hit.Target, hit.TargetType, mode), mode)
	}
	if mode == filterWeaponStrength {
		return namedDamageOption(hit.WeaponStrength, mode)
	}
	return namedDamageOption(hit.WeaponType, mode)
}
func (ui *analyzerUI) damageAttributeGrouping() bool {
	return ui.groupSelect != nil && damageAttributeGroupingMode(ui.groupSelect.Selected)
}

func rebuildDamageFromHits(log *parsedLog) {
	points := make([]damagePoint, 0, min(len(log.Points), len(log.Hits)))
	sources := make([]incomingSourcePoint, 0, min(len(log.IncomingSources), len(log.Hits)))
	sourceIDs := map[int]int{}
	sourceStart := 0
	flush := func() {
		if len(sources)-sourceStart < 2 {
			return
		}
		sort.Slice(sources[sourceStart:], func(i, j int) bool { return sources[sourceStart+i].Source < sources[sourceStart+j].Source })
	}
	for _, hit := range log.Hits {
		if hit.WeaponStrength == "Misses" {
			continue
		}
		if len(points) == 0 || points[len(points)-1].Second != hit.Second {
			flush()
			sourceStart = len(sources)
			clear(sourceIDs)
			points = append(points, damagePoint{Second: hit.Second})
		}
		point := &points[len(points)-1]
		if hit.Incoming {
			point.Incoming += hit.Amount
			point.IncomingHits++
			if hit.Source >= 0 {
				index, exists := sourceIDs[hit.Source]
				if !exists {
					index = len(sources)
					sources = append(sources, incomingSourcePoint{Second: hit.Second, Source: hit.Source})
					sourceIDs[hit.Source] = index
				}
				sources[index].Damage += hit.Amount
				sources[index].Hits++
			}
		} else {
			point.Outgoing += hit.Amount
			point.OutgoingHits++
		}
	}
	flush()
	log.Points, log.IncomingSources = points, sources
}

func sortDamageRecords(log *parsedLog) {
	sort.Slice(log.Points, func(i, j int) bool { return log.Points[i].Second < log.Points[j].Second })
	sort.Slice(log.IncomingSources, func(i, j int) bool {
		a, b := log.IncomingSources[i], log.IncomingSources[j]
		return a.Second < b.Second || a.Second == b.Second && a.Source < b.Source
	})
}
