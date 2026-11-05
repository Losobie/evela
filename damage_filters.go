package main

import (
	"encoding/json"
	"fmt"

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
var damageFilterKinds = append(append([]string(nil), sharedDamageFilterKinds...), filterNPCNames, filterNPCTypes, filterWeaponType, filterWeaponStrength)

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

func selectionLabel(selection *damageFilterSelection, options []damageFilterOption) string {
	count := 0
	for _, option := range options {
		if selection.selected(option.Key) {
			count++
		}
	}
	switch {
	case count == 0 && (len(options) > 0 || selection != nil && !selection.Default):
		return "None"
	case count < len(options):
		return fmt.Sprintf("%d/%d", count, len(options))
	default:
		return "All"
	}
}

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
	for _, candidate := range sharedDamageFilterKinds {
		if candidate == kind {
			return ui.globalFilter(kind).selected(key)
		}
	}
	return ui.scopeFilter(graphIncoming, kind).selected(key)
}

func (ui *analyzerUI) legacyFilterScopes(kind string) []string {
	for _, candidate := range sharedDamageFilterKinds {
		if candidate == kind {
			return []string{globalFilterScope}
		}
	}
	if kind == filterNPCNames || kind == filterNPCTypes {
		return []string{graphIncoming}
	}
	return []string{graphOutgoing, graphIncoming}
}
func (ui *analyzerUI) setDamageFilterOption(kind, key string, checked bool) {
	for _, scope := range ui.legacyFilterScopes(kind) {
		ui.visibleFilters[scope][kind] = true
		ui.storedScopeFilter(scope, kind).set(key, checked)
	}
	if kind == filterCharacters {
		ui.selected[key] = checked
	}
	ui.damageFiltersChanged()
}
func (ui *analyzerUI) setAllDamageFilterOptions(kind string, checked bool) {
	for _, scope := range ui.legacyFilterScopes(kind) {
		ui.visibleFilters[scope][kind] = true
		selection := ui.storedScopeFilter(scope, kind)
		selection.Default, selection.Values = checked, map[string]bool{}
	}
	if kind == filterCharacters {
		for name := range ui.selected {
			ui.selected[name] = checked
		}
	}
	ui.damageFiltersChanged()
}

func (ui *analyzerUI) damageFiltersChanged() {
	ui.saveScopedFilters()
	ui.refreshScopedFilterBars()
	data, err := json.Marshal(ui.damageFilters)
	if err == nil {
		fyne.CurrentApp().Preferences().SetString(damageFiltersPreference, string(data))
	}
	ui.rebuildFilteredView()
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

func characterGroupsSelected(selection *damageFilterSelection, groups []string) bool {
	if len(groups) == 0 {
		return selection.selected(filterUngrouped)
	}
	for _, group := range groups {
		if selection.selected(group) {
			return true
		}
	}
	return false
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
	selections := []*damageFilterSelection{ui.damageFilters[filterSystem]}
	for _, graph := range graphVisibilityOptions {
		selections = append(selections, ui.storedScopeFilter(graph.id, filterSystem))
	}
	changed := false
	for _, selection := range selections {
		if selection == nil {
			continue
		}
		for name, id := range locations.systems {
			key := "system-name:" + name
			if checked, exists := selection.Values[key]; exists {
				selection.set(fmt.Sprintf("system:%d", id), checked)
				delete(selection.Values, key)
				changed = true
			}
		}
	}
	if changed {
		ui.saveScopedFilters()
	}
}

func (ui *analyzerUI) refreshDamageFilterOptions() {
	input := snapshotInRange(ui.snapshot, ui.view.Start, ui.view.End, 0)
	catalog := newGraphFilterCatalog(ui.graphMetadata(), damageFilterKinds)
	for _, group := range ui.characterGroups {
		catalog.add(filterCharacterGroup, damageFilterOption{Key: "group:" + group.ID, Label: group.Name})
	}
	catalog.add(filterCharacterGroup, damageFilterOption{Key: filterUngrouped, Label: ungroupedCharacters})
	ui.scopedOptions = map[string]map[string][]damageFilterOption{}
	for _, graph := range graphVisibilityOptions {
		options := ui.buildScopeFilterOptions(graph.id, input)
		ui.scopedOptions[graph.id] = options
		for kind, values := range options {
			for _, option := range values {
				catalog.add(kind, option)
			}
		}
	}
	for _, log := range input.Logs {
		if len(log.Events) > 0 {
			catalog.add(filterCharacters, damageFilterOption{Key: log.Character, Label: log.Character})
		}
		for _, event := range log.StructureEvents {
			catalog.add(filterSystem, ui.graphMetadata().attributes(log.Character, event.Second).system)
		}
	}
	ui.filterOptions = catalog.options()
	for _, option := range ui.filterOptions[filterCharacters] {
		if _, exists := ui.selected[option.Key]; !exists {
			ui.selected[option.Key] = ui.damageFilterSelected(filterCharacters, option.Key)
		}
	}
}
