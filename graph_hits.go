package main

import (
	"sort"
	"time"
)

type plotHit struct {
	Second    int64
	Damage    float64
	Character string
	Detail    string
}

func (ui *analyzerUI) hitCharacterGroups(character, filterID string) []string {
	var names []string
	for _, group := range ui.characterGroups {
		if filterID != "" && group.ID != filterID {
			continue
		}
		if ui.damageFilters[filterCharacterGroup].selected("group:"+group.ID) && group.contains(character) {
			names = append(names, group.Name)
		}
	}
	if len(names) == 0 && filterID == "" && ui.damageFilters[filterCharacterGroup].selected(filterUngrouped) {
		names = append(names, ungroupedCharacters)
	}
	return names
}

// Keep raw hit amounts and timestamps separate from the bucket rates used by
// totals and averaged graphs. Grouping changes their color/label, never their size.
func (ui *analyzerUI) attachIndividualHits(in, out *plotData, selected map[string]bool) {
	in.IndividualHits, out.IndividualHits = true, true
	incoming, outgoing := map[string][]plotHit{}, map[string][]plotHit{}
	incomingLabels := map[string]string{}
	if ui.incomingGrouping != incomingByCharacter && ui.incomingGrouping != incomingByCharacterGroup {
		for _, series := range in.Series {
			incomingLabels[characterKey(series.Name)] = series.Name
		}
	}
	ships := ui.shipHistory.indexByName()
	first, end := ui.view.Start.Unix(), ui.view.End.Unix()
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		outNames := []string{log.Character}
		if ui.characterGroupGrouping() {
			outNames = ui.hitCharacterGroups(log.Character, ui.selectedCharacterGroupID())
		}
		inNames := []string{log.Character}
		if ui.incomingGrouping == incomingByCharacterGroup {
			inNames = ui.hitCharacterGroups(log.Character, "")
		}
		add := func(hit damageHit) {
			if hit.Second < first || hit.Second >= end {
				return
			}
			point := plotHit{Second: hit.Second, Damage: hit.Amount, Character: log.Character}
			if hit.Incoming {
				names := inNames
				if ui.incomingGrouping != incomingByCharacter && ui.incomingGrouping != incomingByCharacterGroup {
					source := damageSource{}
					if hit.Source >= 0 && hit.Source < len(log.Sources) {
						source = log.Sources[hit.Source]
					}
					names = []string{incomingSourceGroup(source, ui.incomingGrouping).Label}
				}
				for _, name := range names {
					if label, exists := incomingLabels[characterKey(name)]; exists {
						name = label
					}
					incoming[name] = append(incoming[name], point)
				}
			} else {
				names := outNames
				if ui.shipGrouping() {
					ship, known := ships[characterKey(log.Character)].atName(log.Character, time.Unix(hit.Second, 0))
					hull, class := shipFilterOptions(ship, known)
					name := hull.Label
					if ui.shipClassGrouping() {
						name = class.Label
					}
					names = []string{name}
				}
				for _, name := range names {
					outgoing[name] = append(outgoing[name], point)
				}
			}
		}
		if len(log.Hits) > 0 {
			for _, hit := range log.Hits {
				add(hit)
			}
			continue
		}
		// Compatibility with summaries constructed without raw hit records.
		for _, point := range log.Points {
			if point.Outgoing != 0 || point.OutgoingHits > 0 {
				add(damageHit{Second: point.Second, Amount: point.Outgoing, Source: -1})
			}
			if len(log.IncomingSources) == 0 && (point.Incoming != 0 || point.IncomingHits > 0) {
				add(damageHit{Second: point.Second, Amount: point.Incoming, Source: -1, Incoming: true})
			}
		}
		for _, point := range log.IncomingSources {
			add(damageHit{Second: point.Second, Amount: point.Damage, Source: point.Source, Incoming: true})
		}
	}
	attach := func(data *plotData, hits map[string][]plotHit) {
		for index := range data.Series {
			points := hits[data.Series[index].Name]
			sort.SliceStable(points, func(i, j int) bool { return points[i].Second < points[j].Second })
			data.Series[index].Hits = points
		}
	}
	attach(in, incoming)
	attach(out, outgoing)
}
