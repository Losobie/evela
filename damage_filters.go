package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"fyne.io/fyne/v2"
)

const (
	filterCharacters        = "Characters"
	filterShipType          = "Ship Type"
	filterShipClass         = "Ship Class"
	filterCharacterGroup    = "Character Group"
	filterSystem            = "System"
	damageFiltersPreference = "damage.filters.v1"
	filterUngrouped         = "ungrouped"
)

var sharedDamageFilterKinds = []string{filterCharacters, filterShipType, filterShipClass, filterCharacterGroup, filterSystem}
var damageFilterKinds = append(append([]string(nil), sharedDamageFilterKinds...), filterNPCNames, filterNPCTypes)

type damageFilterSelection struct {
	Default bool            `json:"default"`
	Values  map[string]bool `json:"values,omitempty"`
}

func (selection *damageFilterSelection) selected(key string) bool {
	if selection == nil {
		return true
	}
	if value, exists := selection.Values[key]; exists {
		return value
	}
	return selection.Default
}

func (selection *damageFilterSelection) set(key string, checked bool) {
	if selection.Values == nil {
		selection.Values = map[string]bool{}
	}
	if checked == selection.Default {
		delete(selection.Values, key)
	} else {
		selection.Values[key] = checked
	}
}

func (selection *damageFilterSelection) restricted() bool {
	if selection == nil {
		return false
	}
	if !selection.Default {
		return true
	}
	for _, checked := range selection.Values {
		if !checked {
			return true
		}
	}
	return false
}

type damageFilterOption struct{ Key, Label string }

func (ui *analyzerUI) initDamageFilters() {
	var saved map[string]*damageFilterSelection
	if err := json.Unmarshal([]byte(fyne.CurrentApp().Preferences().String(damageFiltersPreference)), &saved); err != nil {
		saved = nil
	}
	ui.damageFilters = map[string]*damageFilterSelection{}
	for _, kind := range damageFilterKinds {
		selection := saved[kind]
		if selection == nil {
			selection = &damageFilterSelection{Default: true}
		}
		ui.damageFilters[kind] = selection
	}
}

func (ui *analyzerUI) damageFilterSelected(kind, key string) bool {
	if kind == filterCharacters {
		if value, exists := ui.selected[key]; exists {
			return value
		}
	}
	return ui.damageFilters[kind].selected(key)
}

func (ui *analyzerUI) setDamageFilterOption(kind, key string, checked bool) {
	ui.damageFilters[kind].set(key, checked)
	if kind == filterCharacters {
		ui.selected[key] = checked
	}
	ui.damageFiltersChanged()
}

func (ui *analyzerUI) setAllDamageFilterOptions(kind string, checked bool) {
	selection := ui.damageFilters[kind]
	selection.Default, selection.Values = checked, map[string]bool{}
	if kind == filterCharacters {
		// All covers the complete option list, including search-hidden options,
		// while preserving character choices outside the current time window.
		for _, option := range ui.filterOptions[kind] {
			ui.selected[option.Key] = checked
		}
		for name, value := range ui.selected {
			selection.set(name, value)
		}
	}
	ui.damageFiltersChanged()
}

func (ui *analyzerUI) damageFiltersChanged() {
	data, err := json.Marshal(ui.damageFilters)
	if err == nil {
		fyne.CurrentApp().Preferences().SetString(damageFiltersPreference, string(data))
	}
	ui.rebuildView()
}

func (ui *analyzerUI) analysisSnapshot() *logSnapshot {
	if ui.analysis != nil {
		return ui.analysis
	}
	return ui.snapshot
}

func shipFilterOptions(ship shipPeriod, known bool) (damageFilterOption, damageFilterOption) {
	hull := damageFilterOption{Key: "unknown", Label: "Unknown ship"}
	class := damageFilterOption{Key: "unknown", Label: "Unknown ship class"}
	if known {
		if ship.TypeID > 0 {
			hull.Key, hull.Label = fmt.Sprintf("type:%d", ship.TypeID), ship.TypeName
			if hull.Label == "" {
				hull.Label = fmt.Sprintf("Type %d", ship.TypeID)
			}
		} else if ship.TypeName != "" {
			hull.Key, hull.Label = "name:"+ship.TypeName, ship.TypeName
		}
		if ship.ClassName != "" {
			class.Key, class.Label = "name:"+ship.ClassName, ship.ClassName
			if ship.GroupID > 0 {
				class.Key = fmt.Sprintf("class:%d", ship.GroupID)
			}
		}
	}
	return hull, class
}

func (ui *analyzerUI) characterFilterGroups() map[string][]string {
	members := map[string][]string{}
	for _, group := range ui.characterGroups {
		for _, name := range group.Members {
			key := characterKey(name)
			members[key] = append(members[key], "group:"+group.ID)
		}
	}
	return members
}

func systemFilterOption(location locationPeriod, known bool) damageFilterOption {
	if !known || (location.SystemID <= 0 && location.SystemName == "") {
		return damageFilterOption{Key: "unknown", Label: "Unknown system"}
	}
	label := location.SystemName
	if location.SystemID <= 0 {
		return damageFilterOption{Key: "system-name:" + characterKey(label), Label: label}
	}
	if label == "" {
		label = fmt.Sprintf("System %d", location.SystemID)
	}
	return damageFilterOption{Key: fmt.Sprintf("system:%d", location.SystemID), Label: label}
}

// Logs supply names without requiring ESI. Preserve selections when a later ESI
// observation supplies the ID for a previously name-only system option.
func (ui *analyzerUI) migrateSystemFilterNames(locations locationResolver) {
	selection := ui.damageFilters[filterSystem]
	if selection == nil {
		return
	}
	changed := false
	for name, id := range locations.systems {
		key := "system-name:" + name
		if checked, exists := selection.Values[key]; exists {
			selection.set(fmt.Sprintf("system:%d", id), checked)
			delete(selection.Values, key)
			changed = true
		}
	}
	if changed {
		if data, err := json.Marshal(ui.damageFilters); err == nil {
			fyne.CurrentApp().Preferences().SetString(damageFiltersPreference, string(data))
		}
	}
}

func (ui *analyzerUI) filteredDamageSnapshot() *logSnapshot {
	shipType, shipClass, groups := ui.damageFilters[filterShipType], ui.damageFilters[filterShipClass], ui.damageFilters[filterCharacterGroup]
	restrictType, restrictClass := shipType.restricted(), shipClass.restricted()
	systems := ui.damageFilters[filterSystem]
	restrictSystem := systems.restricted()
	npcNames, npcTypes := ui.damageFilters[filterNPCNames], ui.damageFilters[filterNPCTypes]
	restrictNPC := npcNames.restricted() || npcTypes.restricted()
	if !restrictType && !restrictClass && !groups.restricted() && !restrictSystem && !restrictNPC {
		return nil
	}
	byName, members := ui.shipHistory.indexByName(), ui.characterFilterGroups()
	locations := ui.locationLookup
	filtered := *ui.snapshot
	filtered.Logs = nil
	for _, log := range ui.snapshot.Logs {
		keys := members[characterKey(log.Character)]
		if len(keys) == 0 {
			keys = []string{filterUngrouped}
		}
		groupMatches := false
		for _, key := range keys {
			groupMatches = groupMatches || groups.selected(key)
		}
		if !groupMatches {
			continue
		}
		matches := func(second int64) bool {
			at := time.Unix(second, 0)
			if restrictSystem {
				location, known := locations.atName(log.Character, at)
				if !systems.selected(systemFilterOption(location, known).Key) {
					return false
				}
			}
			if !restrictType && !restrictClass {
				return true
			}
			ship, known := byName[characterKey(log.Character)].atName(log.Character, at)
			hull, class := shipFilterOptions(ship, known)
			return shipType.selected(hull.Key) && shipClass.selected(class.Key)
		}
		copyLog := *log
		copyLog.Points, copyLog.StructureEvents = nil, nil
		copyLog.IncomingSources = nil
		// Rebuild incoming values from source contributions before bucketing and
		// averaging. Outgoing values and structure markers use only shared filters.
		incoming := map[int64]incomingSourcePoint{}
		included := make([]bool, len(log.Sources))
		for id, source := range log.Sources {
			names, types := npcFilterOptions(source)
			included[id] = npcNames.selected(names.Key) && npcTypes.selected(types.Key)
		}
		for _, contribution := range log.IncomingSources {
			if included[contribution.Source] && matches(contribution.Second) {
				copyLog.IncomingSources = append(copyLog.IncomingSources, contribution)
				if restrictNPC {
					point := incoming[contribution.Second]
					point.Damage += contribution.Damage
					point.Hits += contribution.Hits
					incoming[contribution.Second] = point
				}
			}
		}
		for _, point := range log.Points {
			if matches(point.Second) {
				if restrictNPC {
					if len(log.IncomingSources) == 0 {
						// Unattributed synthetic/legacy data is never guessed to be NPC.
						if !npcNames.selected(filterOtherSources) || !npcTypes.selected(filterOtherSources) {
							point.Incoming, point.IncomingHits = 0, 0
						}
					} else {
						value := incoming[point.Second]
						point.Incoming, point.IncomingHits = value.Damage, value.Hits
					}
				}
				copyLog.Points = append(copyLog.Points, point)
			}
		}
		for _, event := range log.StructureEvents {
			if matches(event.Second) {
				copyLog.StructureEvents = append(copyLog.StructureEvents, event)
			}
		}
		filtered.Logs = append(filtered.Logs, &copyLog)
	}
	return &filtered
}

func (ui *analyzerUI) refreshDamageFilterOptions() {
	options := map[string]map[string]string{}
	for _, kind := range damageFilterKinds {
		options[kind] = map[string]string{}
	}
	for _, group := range ui.characterGroups {
		options[filterCharacterGroup]["group:"+group.ID] = group.Name
	}
	options[filterCharacterGroup][filterUngrouped] = ungroupedCharacters
	byName := ui.shipHistory.indexByName()
	locations := ui.locationLookup
	first, end := ui.view.Start.Unix(), ui.view.End.Unix()
	for _, log := range ui.snapshot.Logs {
		for _, point := range log.IncomingSources {
			if point.Second >= first && point.Second < end {
				names, types := npcFilterOptions(log.Sources[point.Source])
				options[filterNPCNames][names.Key], options[filterNPCTypes][types.Key] = names.Label, types.Label
			}
		}
		for _, point := range log.Points {
			if point.Second < first || point.Second >= end || (point.Incoming == 0 && point.Outgoing == 0 && point.IncomingHits == 0 && point.OutgoingHits == 0) {
				continue
			}
			options[filterCharacters][log.Character] = log.Character
			if len(log.IncomingSources) == 0 && (point.Incoming != 0 || point.IncomingHits != 0) {
				names, types := npcFilterOptions(damageSource{})
				options[filterNPCNames][names.Key], options[filterNPCTypes][types.Key] = names.Label, types.Label
			}
			if _, exists := ui.selected[log.Character]; !exists {
				ui.selected[log.Character] = ui.damageFilterSelected(filterCharacters, log.Character)
			}
			ship, known := byName[characterKey(log.Character)].atName(log.Character, time.Unix(point.Second, 0))
			hull, class := shipFilterOptions(ship, known)
			options[filterShipType][hull.Key], options[filterShipClass][class.Key] = hull.Label, class.Label
			location, known := locations.atName(log.Character, time.Unix(point.Second, 0))
			system := systemFilterOption(location, known)
			options[filterSystem][system.Key] = system.Label
		}
		// Markers can occur after the last damage hit in a system.
		for _, event := range log.StructureEvents {
			if event.Second >= first && event.Second < end {
				location, known := locations.atName(log.Character, time.Unix(event.Second, 0))
				system := systemFilterOption(location, known)
				options[filterSystem][system.Key] = system.Label
			}
		}
	}
	ui.filterOptions = map[string][]damageFilterOption{}
	for _, kind := range damageFilterKinds {
		for key, label := range options[kind] {
			ui.filterOptions[kind] = append(ui.filterOptions[kind], damageFilterOption{Key: key, Label: label})
		}
		sort.Slice(ui.filterOptions[kind], func(i, j int) bool {
			return compareTotalsText(ui.filterOptions[kind][i].Label, ui.filterOptions[kind][j].Label) < 0
		})
	}
}
