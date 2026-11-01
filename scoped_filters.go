package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const globalFilterScope = "global"
const scopedFiltersPreference = "filters.scopes.v1"
const visibleFiltersPreference = "filters.visible.v1"
const filterLogisticsType = "Type"
const filterEffectType = "Effect Type"
const filterAttacker = "Attacker"
const filterAttackerShip = "Attacker Ship"

// This catalog is shared by the Add filter menu and the filtering engine.
func availableFilters(scope string) []string {
	result := append([]string(nil), sharedDamageFilterKinds...)
	switch scope {
	case graphOutgoing:
		result = append(result, filterWeaponType, filterWeaponStrength)
	case graphIncoming:
		result = append(result, filterWeaponType, filterWeaponStrength, filterAttacker, filterAttackerShip, filterNPCNames, filterNPCTypes)
	case graphOutgoingLogistics, graphIncomingLogistics:
		result = append(result, filterLogisticsType)
	case graphIncomingEWar:
		result = append(result, filterEffectType)
	case graphMining:
		result = append(result, miningFilterKinds...)
	}
	return result
}

func (ui *analyzerUI) initScopedFilters() {
	_ = json.Unmarshal([]byte(fyne.CurrentApp().Preferences().String(scopedFiltersPreference)), &ui.scopedFilters)
	if ui.scopedFilters == nil {
		ui.scopedFilters = map[string]map[string]*damageFilterSelection{}
	}
	savedVisibility := fyne.CurrentApp().Preferences().String(visibleFiltersPreference)
	_ = json.Unmarshal([]byte(savedVisibility), &ui.visibleFilters)
	if ui.visibleFilters == nil {
		ui.visibleFilters = map[string]map[string]bool{}
	}
	scopes := []string{globalFilterScope}
	for _, graph := range graphVisibilityOptions {
		scopes = append(scopes, graph.id)
	}
	for _, scope := range scopes {
		if ui.scopedFilters[scope] == nil {
			ui.scopedFilters[scope] = map[string]*damageFilterSelection{}
		}
		if ui.visibleFilters[scope] == nil {
			ui.visibleFilters[scope] = map[string]bool{}
		}
		for _, kind := range availableFilters(scope) {
			if ui.scopedFilters[scope][kind] == nil {
				selection := &damageFilterSelection{Default: true}
				// Preserve previously active restrictions, without cluttering new installs.
				if scope == graphIncoming && (kind == filterNPCNames || kind == filterNPCTypes) || (scope == graphIncoming || scope == graphOutgoing) && weaponGroupingMode(kind) {
					data, _ := json.Marshal(ui.damageFilters[kind])
					_ = json.Unmarshal(data, &selection)
				}
				ui.scopedFilters[scope][kind] = selection
			}
			if savedVisibility == "" && ui.storedScopeFilter(scope, kind).restricted() {
				ui.visibleFilters[scope][kind] = true
			}
		}
	}
	ui.scopedFilterBars = map[string]*fyne.Container{}
	ui.scopedFilterButtons = map[string]map[string]*widget.Button{}
	ui.filterButtons = map[string]*widget.Button{}
}

func (ui *analyzerUI) storedScopeFilter(scope, kind string) *damageFilterSelection {
	if scope == globalFilterScope {
		return ui.damageFilters[kind]
	}
	if kind == filterLogisticsType {
		return ui.logisticsTypes[scope]
	}
	if scope == graphMining && slices.Contains(miningFilterKinds, kind) {
		return ui.miningFilters[kind]
	}
	return ui.scopedFilters[scope][kind]
}
func (ui *analyzerUI) scopeFilter(scope, kind string) *damageFilterSelection {
	if !ui.visibleFilters[scope][kind] {
		return nil
	}
	return ui.storedScopeFilter(scope, kind)
}
func (ui *analyzerUI) globalFilter(kind string) *damageFilterSelection {
	return ui.scopeFilter(globalFilterScope, kind)
}

func (ui *analyzerUI) saveScopedFilters() {
	for key, value := range map[string]any{scopedFiltersPreference: ui.scopedFilters, visibleFiltersPreference: ui.visibleFilters, damageFiltersPreference: ui.damageFilters, miningFiltersPreference: ui.miningFilters} {
		data, _ := json.Marshal(value)
		fyne.CurrentApp().Preferences().SetString(key, string(data))
	}
	for id, value := range ui.logisticsTypes {
		data, _ := json.Marshal(value)
		fyne.CurrentApp().Preferences().SetString("graph.logistics.types."+id, string(data))
	}
}
func (ui *analyzerUI) setFilterVisible(scope, kind string, visible bool) {
	if !slices.Contains(availableFilters(scope), kind) {
		return
	}
	ui.visibleFilters[scope][kind] = visible
	if scope == globalFilterScope && kind == filterCharacters && visible {
		for name := range ui.selected {
			ui.selected[name] = ui.damageFilters[filterCharacters].selected(name)
		}
	}
	if scope == globalFilterScope && kind == filterCharacters && !visible {
		// Character checkboxes and the global Characters popup share one selection.
		for name := range ui.selected {
			ui.selected[name] = true
		}
	}
	if ui.filterPopup != nil {
		ui.filterPopup.popup.Hide()
	}
	ui.saveScopedFilters()
	ui.refreshScopedFilterBars()
	ui.rebuildView()
}
func (ui *analyzerUI) setScopeFilter(scope, kind, key string, checked bool) {
	ui.storedScopeFilter(scope, kind).set(key, checked)
	if scope == globalFilterScope && kind == filterCharacters {
		ui.selected[key] = checked
	}
	ui.saveScopedFilters()
	ui.rebuildView()
}
func (ui *analyzerUI) setAllScopeFilters(scope, kind string, checked bool) {
	selection := ui.storedScopeFilter(scope, kind)
	selection.Default, selection.Values = checked, map[string]bool{}
	if scope == globalFilterScope && kind == filterCharacters {
		for name := range ui.selected {
			ui.selected[name] = checked
		}
	}
	ui.saveScopedFilters()
	ui.rebuildView()
}

func (ui *analyzerUI) scopedFilterBar(scope string) fyne.CanvasObject {
	bar := container.NewVBox()
	ui.scopedFilterBars[scope] = bar
	ui.refreshScopedFilterBar(scope)
	return bar
}
func (ui *analyzerUI) addFilterMenu(scope string) *fyne.Menu {
	menu := fyne.NewMenu("Add filter")
	for _, kind := range availableFilters(scope) {
		if ui.visibleFilters[scope][kind] {
			continue
		}
		kind := kind
		menu.Items = append(menu.Items, fyne.NewMenuItem(kind, func() { ui.setFilterVisible(scope, kind, true) }))
	}
	if len(menu.Items) == 0 {
		item := fyne.NewMenuItem("All filters added", nil)
		item.Disabled = true
		menu.Items = append(menu.Items, item)
	}
	return menu
}
func (ui *analyzerUI) refreshScopedFilterBars() {
	for scope := range ui.scopedFilterBars {
		ui.refreshScopedFilterBar(scope)
	}
}
func (ui *analyzerUI) refreshScopedFilterBar(scope string) {
	bar := ui.scopedFilterBars[scope]
	if bar == nil {
		return
	}
	ui.scopedFilterButtons[scope] = map[string]*widget.Button{}
	chips := container.NewHBox()
	label := "Graph filters"
	if scope == globalFilterScope {
		label = "Global filters"
	}
	for _, kind := range availableFilters(scope) {
		if !ui.visibleFilters[scope][kind] {
			continue
		}
		kind := kind
		var button *widget.Button
		button = widget.NewButtonWithIcon(ui.scopeFilterLabel(scope, kind), theme.MenuDropDownIcon(), func() { ui.openScopedFilter(scope, kind, button) })
		ui.scopedFilterButtons[scope][kind] = button
		// Legacy references remain useful for existing character controls.
		if scope == globalFilterScope || scope == graphIncoming && !slices.Contains(sharedDamageFilterKinds, kind) {
			ui.filterButtons[kind] = button
		}
		if kind == filterLogisticsType {
			ui.logisticsTypeButtons[scope] = button
		}
		if scope == graphMining && slices.Contains(miningFilterKinds, kind) {
			ui.miningFilterButtons[kind] = button
		}
		remove := widget.NewButtonWithIcon("", theme.CancelIcon(), func() { ui.setFilterVisible(scope, kind, false) })
		chips.Add(container.NewHBox(button, remove))
	}
	add := ui.graphMenuButton("Add filter", func() *fyne.Menu { return ui.addFilterMenu(scope) })
	bar.Objects = []fyne.CanvasObject{container.NewBorder(nil, nil, widget.NewLabel(label), add, container.NewHScroll(chips))}
	bar.Refresh()
}
func (ui *analyzerUI) scopeFilterLabel(scope, kind string) string {
	selection := ui.storedScopeFilter(scope, kind)
	options := ui.scopeFilterOptions(scope, kind)
	count := 0
	for _, option := range options {
		if selection.selected(option.Key) {
			count++
		}
	}
	label := "All"
	if count == 0 && (len(options) > 0 || !selection.Default) {
		label = "None"
	} else if count < len(options) {
		label = fmt.Sprintf("%d/%d", count, len(options))
	}
	return kind + ": " + label
}
func (ui *analyzerUI) openScopedFilter(scope, kind string, anchor fyne.CanvasObject) {
	ui.openSelectionFilter(kind, anchor, func() []damageFilterOption { return ui.scopeFilterOptions(scope, kind) },
		func(key string) bool { return ui.storedScopeFilter(scope, kind).selected(key) },
		func(key string, checked bool) { ui.setScopeFilter(scope, kind, key, checked) },
		func(checked bool) { ui.setAllScopeFilters(scope, kind, checked) })
}

func (ui *analyzerUI) scopeFilterOptions(scope, kind string) []damageFilterOption {
	if scope == globalFilterScope {
		return ui.filterOptions[kind]
	}
	return ui.scopedOptions[scope][kind]
}

func (ui *analyzerUI) buildScopeFilterOptions(scope string) map[string][]damageFilterOption {
	values := map[string]map[string]string{}
	for _, kind := range availableFilters(scope) {
		values[kind] = map[string]string{}
	}

	first, end := ui.view.Start.Unix(), ui.view.End.Unix()
	ships, members := ui.shipHistory.indexByName(), ui.characterFilterGroups()
	register := func(log *parsedLog, second int64, hit *damageHit, kindValue, ore string) {
		if second < first || second >= end {
			return
		}
		for _, kind := range availableFilters(scope) {
			add := func(option damageFilterOption) { values[kind][option.Key] = option.Label }
			switch kind {
			case filterCharacters:
				add(damageFilterOption{Key: log.Character, Label: log.Character})
			case filterCharacterGroup:
				if len(members[characterKey(log.Character)]) == 0 {
					add(damageFilterOption{Key: filterUngrouped, Label: ungroupedCharacters})
				}
				for _, group := range ui.characterGroups {
					if group.contains(log.Character) {
						add(damageFilterOption{Key: "group:" + group.ID, Label: group.Name})
					}
				}
			case filterShipType, filterShipClass:
				ship, known := ships[characterKey(log.Character)].atName(log.Character, time.Unix(second, 0))
				hull, class := shipFilterOptions(ship, known)
				if kind == filterShipType {
					add(hull)
				} else {
					add(class)
				}
			case filterSystem:
				location, known := ui.locationLookup.atName(log.Character, time.Unix(second, 0))
				add(systemFilterOption(location, known))
			case filterWeaponType, filterWeaponStrength:
				if hit != nil {
					add(hitWeaponOption(*hit, kind))
				}
			case filterAttacker, filterAttackerShip, filterNPCNames, filterNPCTypes:
				source := damageSource{}
				if hit != nil && hit.Source >= 0 && hit.Source < len(log.Sources) {
					source = log.Sources[hit.Source]
				}
				add(sourceFilterOption(source, kind))
			case filterLogisticsType:
				for _, option := range logisticsTypeOptions {
					if option.Key == kindValue {
						add(option)
					}
				}
			case filterEffectType, filterMiningMessage:
				add(damageFilterOption{Key: kindValue, Label: kindValue})
			case filterOreName:
				add(damageFilterOption{Key: ore, Label: ore})
			case filterOreType:
				add(damageFilterOption{Key: oreType(ore), Label: oreType(ore)})
			}
		}
	}
	for _, log := range ui.snapshot.Logs {
		switch scope {
		case graphOutgoing, graphIncoming:
			incoming := scope == graphIncoming
			for _, hit := range log.Hits {
				if hit.Incoming == incoming {
					register(log, hit.Second, &hit, "", "")
				}
			}
			if len(log.Hits) == 0 {
				for _, point := range log.Points {
					if incoming && (point.Incoming != 0 || point.IncomingHits != 0) || !incoming && (point.Outgoing != 0 || point.OutgoingHits != 0) {
						register(log, point.Second, nil, "", "")
					}
				}
			}
		case graphIncomingEWar:
			for _, event := range log.EWar {
				register(log, event.Second, nil, event.Effect, "")
			}
		case graphMining:
			for _, event := range log.Mining {
				register(log, event.Second, nil, event.Kind, event.Ore)
			}
		case graphIncomingLogistics, graphOutgoingLogistics:
			for _, event := range log.Logistics {
				if event.Incoming == (scope == graphIncomingLogistics) {
					register(log, event.Second, nil, event.Kind, "")
				}
			}
		}
	}
	result := map[string][]damageFilterOption{}
	for kind, items := range values {
		for key, label := range items {
			result[kind] = append(result[kind], damageFilterOption{Key: key, Label: label})
		}
		sort.Slice(result[kind], func(i, j int) bool { return compareTotalsText(result[kind][i].Label, result[kind][j].Label) < 0 })
	}
	if scope == graphIncomingLogistics || scope == graphOutgoingLogistics {
		result[filterLogisticsType] = logisticsTypeOptions
	}
	return result
}
func sourceFilterOption(source damageSource, kind string) damageFilterOption {
	switch kind {
	case filterAttacker:
		return incomingSourceGroup(source, incomingByAttacker)
	case filterAttackerShip:
		return incomingSourceGroup(source, incomingByShip)
	case filterNPCNames:
		name, _ := npcFilterOptions(source)
		return name
	default:
		_, typ := npcFilterOptions(source)
		return typ
	}
}

func (ui *analyzerUI) syncCharacterFilter() {
	selection := ui.damageFilters[filterCharacters]
	for name, checked := range ui.selected {
		selection.set(name, checked)
		if !checked {
			ui.visibleFilters[globalFilterScope][filterCharacters] = true
		}
	}
	ui.saveScopedFilters()
	ui.refreshScopedFilterBars()
}
