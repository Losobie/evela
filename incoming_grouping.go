package main

import (
	"math"
	"sort"
	"time"

	"fyne.io/fyne/v2"
)

const (
	incomingByCharacter      = "Character"
	incomingByCharacterGroup = "Character Group"
	incomingByAttacker       = "Attacker"
	incomingByShip           = "Ship"
	incomingByNPC            = "NPC"
	incomingByNPCType        = "NPC Type"
)

var incomingGroupingModes = []string{
	incomingByCharacter, incomingByCharacterGroup,
	incomingByAttacker, incomingByShip, incomingByNPC, incomingByNPCType,
}

func (ui *analyzerUI) incomingGroupingMenu() *fyne.Menu {
	menu := fyne.NewMenu("Group By")
	for _, mode := range incomingGroupingModes {
		grouping := mode
		item := fyne.NewMenuItem(grouping, func() {
			ui.incomingGrouping = grouping
			ui.saveGraphFormatPreferences()
			ui.renderSelection()
		})
		item.Checked = grouping == ui.incomingGrouping
		menu.Items = append(menu.Items, item)
	}
	return menu
}

func (ui *analyzerUI) incomingGraphSeries(selected map[string]bool) []characterSeries {
	if ui.incomingGrouping != incomingByCharacter && ui.incomingGrouping != incomingByCharacterGroup {
		return ui.view.incomingSourceSeries(ui.analysisSnapshot(), selected, ui.incomingGrouping, ui.graphAverage10s)
	}
	series := ui.view.activeSeries()
	if ui.graphAverage10s {
		series = ui.view.tenSecondAverage(ui.analysisSnapshot())
	}
	if ui.incomingGrouping == incomingByCharacterGroup {
		return ui.groupCharacterSeriesFor(series, selected, "")
	}
	var result []characterSeries
	for _, character := range series {
		if selected[character.Name] {
			result = append(result, character)
		}
	}
	return result
}

func incomingSourceGroup(source damageSource, mode string) damageFilterOption {
	other := damageFilterOption{Key: "other", Label: "Other / unresolved"}
	switch mode {
	case incomingByAttacker:
		if source.Hull != "" {
			return damageFilterOption{Key: "ship:" + characterKey(source.Name), Label: source.Name}
		}
	case incomingByShip:
		if source.Hull != "" {
			return damageFilterOption{Key: "hull:" + characterKey(source.Hull), Label: source.Hull}
		}
	case incomingByNPC, incomingByNPCType:
		name, group := npcFilterOptions(source)
		if mode == incomingByNPC {
			return name
		}
		return group
	}
	return other
}

// Source attribution precedes bucketing and averaging. Every source contributes
// to one group, including sources without enough information for the chosen mode.
func (view damageView) incomingSourceSeries(snapshot *logSnapshot, selected map[string]bool, mode string, average bool) []characterSeries {
	if view.Bucket < time.Second || !view.End.After(view.Start) {
		return nil
	}
	active := map[string]bool{}
	for _, series := range view.activeSeries() {
		active[series.Name] = selected[series.Name]
	}
	type groupData struct {
		series  characterSeries
		members map[string]bool
		bins    map[int]map[string]bool
	}
	groups := map[string]*groupData{}
	first, end := view.Start.Unix(), view.End.Unix()
	bucket := int64(view.Bucket / time.Second)
	n := int(math.Ceil(view.End.Sub(view.Start).Seconds() / view.Bucket.Seconds()))
	window := int64(1)
	if average {
		window = int64(dpsAverageWindow / time.Second)
	}
	add := func(character string, source damageSource, point incomingSourcePoint) {
		left, right := max(first, point.Second), min(end, point.Second+window)
		if left >= right || (point.Damage == 0 && point.Hits == 0) {
			return
		}
		option := incomingSourceGroup(source, mode)
		group := groups[option.Key]
		if group == nil {
			group = &groupData{series: characterSeries{Name: option.Label, Incoming: make([]float64, n), MemberCounts: make([]int, n)}, members: map[string]bool{}, bins: map[int]map[string]bool{}}
			groups[option.Key] = group
		}
		group.members[character] = true
		if point.Second >= first && point.Second < end {
			group.series.TotalIncoming += point.Damage
			group.series.IncomingHits += point.Hits
		}
		for bin := int((left - first) / bucket); bin <= int((right-1-first)/bucket); bin++ {
			binStart := first + int64(bin)*bucket
			binEnd := min(end, binStart+bucket)
			overlap := min(right, binEnd) - max(left, binStart)
			group.series.Incoming[bin] += point.Damage * float64(overlap) / float64(window) / float64(binEnd-binStart)
			if group.bins[bin] == nil {
				group.bins[bin] = map[string]bool{}
			}
			group.bins[bin][character] = true
		}
	}
	for _, log := range snapshot.Logs {
		if !active[log.Character] {
			continue
		}
		if len(log.IncomingSources) == 0 {
			for _, point := range log.Points {
				add(log.Character, damageSource{}, incomingSourcePoint{Second: point.Second, Damage: point.Incoming, Hits: point.IncomingHits})
			}
		} else {
			for _, point := range log.IncomingSources {
				add(log.Character, log.Sources[point.Source], point)
			}
		}
	}
	var result []characterSeries
	for _, group := range groups {
		for name := range group.members {
			group.series.Members = append(group.series.Members, name)
		}
		sort.Strings(group.series.Members)
		for bin, members := range group.bins {
			group.series.MemberCounts[bin] = len(members)
		}
		result = append(result, group.series)
	}
	sort.Slice(result, func(i, j int) bool { return compareTotalsText(result[i].Name, result[j].Name) < 0 })
	return result
}
