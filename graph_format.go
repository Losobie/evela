package main

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	graphAveragePreference         = "graph.format.average-10s"
	graphGroupingPreference        = "graph.format.grouping"
	graphInvulnerabilityPreference = "graph.format.invulnerability-markers"
	incomingGroupingPreference     = "graph.incoming.grouping"
)

func (ui *analyzerUI) initGraphFormat() {
	preferences := fyne.CurrentApp().Preferences()
	ui.graphAverage10s = preferences.BoolWithFallback(graphAveragePreference, true)
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
	ui.graphFormatLabel = widget.NewLabel("")
	ui.graphFormatLabel.Wrapping = fyne.TextWrapWord
	ui.graphFormatLabel.Truncation = fyne.TextTruncateEllipsis
	ui.graphFormatButton = ui.graphMenuButton("Group By", ui.graphFormatMenu)
	ui.graphOptionsButton = ui.graphMenuButton("Options", ui.graphOptionsMenu)
	ui.incomingGroupButton = ui.graphMenuButton("Group By", ui.incomingGroupingMenu)
	ui.updateGraphFormatLabel()
}

func (ui *analyzerUI) graphMenuButton(label string, items func() *fyne.Menu) *widget.Button {
	var button *widget.Button
	button = widget.NewButtonWithIcon(label, theme.MenuDropDownIcon(), func() {
		if ui.graphMenuPopup != nil {
			ui.graphMenuPopup.Hide()
		}
		menu := widget.NewPopUpMenu(items(), ui.window.Canvas())
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
	preferences.SetBool(graphAveragePreference, ui.graphAverage10s)
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
	return fyne.NewMenu("Group By", characters, ships, classes, groups)
}

func (ui *analyzerUI) graphOptionsMenu() *fyne.Menu {
	average := fyne.NewMenuItem("10-second DPS average", func() {
		ui.graphAverage10s = !ui.graphAverage10s
		ui.saveGraphFormatPreferences()
		ui.renderSelection()
		ui.rebuildCharacters()
	})
	average.Checked = ui.graphAverage10s
	invulnerability := fyne.NewMenuItem("Target invulnerability indicators", func() {
		ui.graphInvulnerabilityMarkers = !ui.graphInvulnerabilityMarkers
		ui.saveGraphFormatPreferences()
		ui.renderSelection()
	})
	invulnerability.Checked = ui.graphInvulnerabilityMarkers
	items := []*fyne.MenuItem{average, invulnerability, fyne.NewMenuItemSeparator()}
	return fyne.NewMenu("Options", append(items, ui.graphVisibilityMenuItems()...)...)
}

func (ui *analyzerUI) updateGraphFormatLabel() {
	if ui.graphFormatLabel == nil {
		return
	}
	grouping := "By character"
	if ui.shipGrouping() {
		grouping = "By ship type · history at event time"
	}
	if ui.shipClassGrouping() {
		grouping = "By ship class · history at event time"
	}
	if ui.characterGroupGrouping() {
		grouping = "By character group"
		if ui.selectedCharacterGroupID() != "" {
			grouping = ui.groupSelect.Selected
		}
	}
	text := "Linear · " + grouping
	if ui.graphAverage10s {
		text += " · 10-second average"
	} else {
		text += " · individual hits"
	}
	ui.graphFormatLabel.SetText(text)
}
