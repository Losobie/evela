package main

import (
	"encoding/json"
	"slices"

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
		result = append(result, filterWeaponType, filterWeaponStrength, filterTarget, filterTargetType)
	case graphIncoming:
		result = append(result, filterWeaponType, filterWeaponStrength, filterAttacker, filterAttackerShip, filterNPCNames, filterNPCTypes)
	case graphOutgoingLogistics:
		result = append(result, filterLogisticsType, filterTarget, filterTargetType)
	case graphIncomingLogistics:
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
				if scope == graphIncoming && (kind == filterNPCNames || kind == filterNPCTypes) || (scope == graphIncoming || scope == graphOutgoing) && (kind == filterWeaponType || kind == filterWeaponStrength) {
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
	ui.rebuildFilteredView()
}
func (ui *analyzerUI) setScopeFilter(scope, kind, key string, checked bool) {
	ui.storedScopeFilter(scope, kind).set(key, checked)
	if scope == globalFilterScope && kind == filterCharacters {
		ui.selected[key] = checked
	}
	ui.saveScopedFilters()
	ui.rebuildFilteredView()
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
	ui.rebuildFilteredView()
}

func (ui *analyzerUI) scopedFilterBar(scope string, trailing ...fyne.CanvasObject) fyne.CanvasObject {
	toolbar := &filterBarLayout{}
	if len(trailing) > 0 {
		toolbar.trailing = trailing[0]
	}
	bar := container.New(toolbar)
	toolbar.owner = bar
	ui.scopedFilterBars[scope] = bar
	ui.refreshScopedFilterBar(scope)
	return bar
}
func (ui *analyzerUI) addFilterMenu(scope string) *fyne.Menu {
	menu := fyne.NewMenu("Add filter")
	for _, kind := range availableFilters(scope) {
		kind := kind
		item := fyne.NewMenuItem(kind, func() { ui.setFilterVisible(scope, kind, !ui.visibleFilters[scope][kind]) })
		item.Checked = ui.visibleFilters[scope][kind]
		menu.Items = append(menu.Items, item)
	}
	return menu
}

func (ui *analyzerUI) keepAddFilterMenuOpen(scope string, popup *widget.PopUpMenu, model *fyne.Menu) {
	for index, item := range model.Items {
		action := item.Action
		item.Action = func() {
			action()
			item.Checked = ui.visibleFilters[scope][item.Label]
			popup.ShowAtPosition(popup.Position())
			for range model.Items[:index+1] {
				popup.ActivateNext()
			}
		}
	}
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
	var chips []fyne.CanvasObject
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
		chips = append(chips, container.NewHBox(button, remove))
	}
	addLabel := "Add filter"
	if scope == globalFilterScope {
		addLabel = "Add global filter"
	}
	add := ui.toggleMenuButton(addLabel, func() *fyne.Menu { return ui.addFilterMenu(scope) }, func(popup *widget.PopUpMenu, model *fyne.Menu) { ui.keepAddFilterMenuOpen(scope, popup, model) })
	bar.Objects = append([]fyne.CanvasObject{add}, chips...)
	if trailing := bar.Layout.(*filterBarLayout).trailing; trailing != nil {
		bar.Objects = append(bar.Objects, trailing)
	}
	bar.Refresh()
}
func (ui *analyzerUI) scopeFilterLabel(scope, kind string) string {
	selection := ui.storedScopeFilter(scope, kind)
	options := ui.scopeFilterOptions(scope, kind)
	return kind + ": " + selectionLabel(selection, options)
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
