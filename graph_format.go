package main

import (
	"fmt"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	graphAveragePreference         = "graph.format.average-10s"
	graphAverageSecondsPreference  = "graph.format.average-seconds"
	graphGroupingPreference        = "graph.format.grouping"
	graphInvulnerabilityPreference = "graph.format.invulnerability-markers"
	incomingGroupingPreference     = "graph.incoming.grouping"
)

func (ui *analyzerUI) initGraphFormat() {
	preferences := fyne.CurrentApp().Preferences()
	legacySeconds := 0
	if preferences.BoolWithFallback(graphAveragePreference, true) {
		legacySeconds = int(dpsAverageWindow / time.Second)
	}
	seconds := preferences.IntWithFallback(graphAverageSecondsPreference, legacySeconds)
	if seconds != 0 && seconds != 10 && seconds != 30 {
		seconds = legacySeconds
	}
	ui.graphAverageWindow = time.Duration(seconds) * time.Second
	ui.graphInvulnerabilityMarkers = preferences.Bool(graphInvulnerabilityPreference)
	ui.incomingGrouping = preferences.String(incomingGroupingPreference)
	switch ui.incomingGrouping {
	case "Attacking Ship":
		ui.incomingGrouping = incomingByAttacker
	case "Attacking Ship Type":
		ui.incomingGrouping = incomingByShip
	}
	if !slices.Contains(incomingGroupingModes, ui.incomingGrouping) {
		ui.incomingGrouping = incomingByCharacter
	}
	// Restore before the first view is built. Assigning directly avoids firing
	// grouping callbacks while the totals table is still being constructed.
	switch preferences.String(graphGroupingPreference) {
	case filterWeaponType, filterWeaponStrength, filterTarget, filterTargetType, filterSystem:
		ui.groupSelect.Selected = preferences.String(graphGroupingPreference)
	case "ship-type":
		ui.groupSelect.Selected = "Ship history"
	case "ship-class":
		ui.groupSelect.Selected = "Ship class history"
	case "character-groups":
		ui.groupSelect.Selected = characterGroupsMode
	default:
		ui.groupSelect.Selected = "Characters"
		for _, group := range ui.characterGroups {
			if preferences.String(graphGroupingPreference) == "character-group:"+group.ID {
				ui.groupSelect.Selected = "Group: " + group.Name
			}
		}
	}
	ui.graphFormatButton = ui.graphMenuButton("Group By", ui.graphFormatMenu)
	ui.graphOptionsButton = ui.graphMenuButton("Options", ui.graphOptionsMenu, true)
	ui.incomingGroupButton = ui.graphMenuButton("Group By", ui.incomingGroupingMenu)
}

func (ui *analyzerUI) graphMenuButton(label string, items func() *fyne.Menu, keepVisibilityOpen ...bool) *widget.Button {
	var keepOpen func(*widget.PopUpMenu, *fyne.Menu)
	if len(keepVisibilityOpen) > 0 && keepVisibilityOpen[0] {
		keepOpen = ui.keepGraphVisibilityMenuOpen
	}
	return ui.toggleMenuButton(label, items, keepOpen)
}

func (ui *analyzerUI) toggleMenuButton(label string, items func() *fyne.Menu, keepOpen func(*widget.PopUpMenu, *fyne.Menu)) *widget.Button {
	var button *widget.Button
	button = widget.NewButtonWithIcon(label, theme.MenuDropDownIcon(), func() {
		if ui.graphMenuPopup != nil {
			ui.graphMenuPopup.Hide()
		}
		model := items()
		sticky := keepOpen != nil
		// Fyne reserves checkmark space only when the menu is constructed.
		// Toggle menus need that column even when every item starts unchecked.
		var reserveCheckColumn *fyne.MenuItem
		if sticky && !slices.ContainsFunc(model.Items, func(item *fyne.MenuItem) bool { return item.Checked }) {
			reserveCheckColumn = model.Items[0]
			reserveCheckColumn.Checked = true
		}
		menu := widget.NewPopUpMenu(model, ui.window.Canvas())
		if reserveCheckColumn != nil {
			reserveCheckColumn.Checked = false
		}
		if sticky {
			keepOpen(menu, model)
		}
		ui.graphMenuPopup = menu
		position := fyne.CurrentApp().Driver().AbsolutePositionForObject(button)
		size, bounds := menu.MinSize(), ui.window.Canvas().Size()
		position.X += button.Size().Width - size.Width
		position.Y += button.Size().Height
		if position.Y+size.Height > bounds.Height && position.Y-button.Size().Height >= size.Height {
			position.Y -= button.Size().Height + size.Height
		}
		position.X = max(0, min(position.X, bounds.Width-size.Width))
		position.Y = max(0, min(position.Y, bounds.Height-size.Height))
		menu.ShowAtPosition(position)
	})
	return button
}

func (ui *analyzerUI) saveGraphFormatPreferences() {
	preferences := fyne.CurrentApp().Preferences()
	preferences.SetInt(graphAverageSecondsPreference, int(ui.graphAverageWindow/time.Second))
	preferences.SetBool(graphInvulnerabilityPreference, ui.graphInvulnerabilityMarkers)
	grouping := "characters"
	if ui.shipClassGrouping() {
		grouping = "ship-class"
	} else if ui.shipGrouping() {
		grouping = "ship-type"
	} else if id := ui.selectedCharacterGroupID(); id != "" {
		grouping = "character-group:" + id
	} else if ui.characterGroupGrouping() {
		grouping = "character-groups"
	}
	if ui.damageAttributeGrouping() || ui.systemGrouping() {
		grouping = ui.groupSelect.Selected
	}
	preferences.SetString(graphGroupingPreference, grouping)
	preferences.SetString(incomingGroupingPreference, ui.incomingGrouping)
}

func (ui *analyzerUI) graphFormatMenu() *fyne.Menu {
	characters := fyne.NewMenuItem("Character", func() { ui.groupSelect.SetSelected("Characters") })
	characters.Checked = !ui.anyGrouping()
	ships := fyne.NewMenuItem("Ship type (ship history)", func() { ui.groupSelect.SetSelected("Ship history") })
	ships.Checked = ui.shipGrouping() && !ui.shipClassGrouping()
	classes := fyne.NewMenuItem("Ship class (ship history)", func() { ui.groupSelect.SetSelected("Ship class history") })
	classes.Checked = ui.shipClassGrouping()
	groups := fyne.NewMenuItem("Character Group", func() { ui.groupSelect.SetSelected(characterGroupsMode) })
	groups.Checked = ui.groupSelect.Selected == characterGroupsMode
	menu := fyne.NewMenu("Group By", characters, ships, classes, groups)
	for _, mode := range []string{filterWeaponType, filterWeaponStrength, filterTarget, filterTargetType, filterSystem} {
		mode := mode
		item := fyne.NewMenuItem(mode, func() { ui.groupSelect.SetSelected(mode) })
		item.Checked = ui.groupSelect.Selected == mode
		menu.Items = append(menu.Items, item)
	}
	return menu
}

func (ui *analyzerUI) graphOptionsMenu() *fyne.Menu {
	items := []*fyne.MenuItem{}
	for _, window := range []time.Duration{10 * time.Second, 30 * time.Second} {
		averageWindow := window
		average := fyne.NewMenuItem(fmt.Sprintf("%d-second DPS average", int(window/time.Second)), func() {
			if ui.graphAverageWindow == averageWindow {
				ui.graphAverageWindow = 0
			} else {
				ui.graphAverageWindow = averageWindow
			}
			ui.saveGraphFormatPreferences()
			ui.renderSelection()
			ui.rebuildCharacters()
		})
		average.Checked = ui.graphAverageWindow == averageWindow
		items = append(items, average)
	}
	invulnerability := fyne.NewMenuItem("Target invulnerability indicators", func() {
		ui.graphInvulnerabilityMarkers = !ui.graphInvulnerabilityMarkers
		ui.saveGraphFormatPreferences()
		ui.renderSelection()
	})
	invulnerability.Checked = ui.graphInvulnerabilityMarkers
	items = append(items, invulnerability, fyne.NewMenuItemSeparator())
	return fyne.NewMenu("Options", append(items, ui.graphVisibilityMenuItems()...)...)
}
