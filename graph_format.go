package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	graphLogarithmicPreference = "graph.format.logarithmic"
	graphAveragePreference     = "graph.format.average-10s"
	graphGroupingPreference    = "graph.format.grouping"
)

func (ui *analyzerUI) initGraphFormat() {
	preferences := fyne.CurrentApp().Preferences()
	ui.graphLogarithmic = preferences.Bool(graphLogarithmicPreference)
	ui.graphAverage10s = preferences.Bool(graphAveragePreference)
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
	ui.graphFormatButton = widget.NewButtonWithIcon("Format options", theme.MenuDropDownIcon(), func() {
		menu := widget.NewPopUpMenu(ui.graphFormatMenu(), ui.window.Canvas())
		menu.ShowAtRelativePosition(fyne.NewPos(ui.graphFormatButton.Size().Width-menu.MinSize().Width,
			ui.graphFormatButton.Size().Height), ui.graphFormatButton)
	})
	ui.updateGraphFormatLabel()
}

func (ui *analyzerUI) saveGraphFormatPreferences() {
	preferences := fyne.CurrentApp().Preferences()
	preferences.SetBool(graphLogarithmicPreference, ui.graphLogarithmic)
	preferences.SetBool(graphAveragePreference, ui.graphAverage10s)
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
}

func (ui *analyzerUI) graphFormatMenu() *fyne.Menu {
	linear := fyne.NewMenuItem("Linear", func() {
		ui.graphLogarithmic = false
		ui.saveGraphFormatPreferences()
		ui.renderSelection()
	})
	linear.Checked = !ui.graphLogarithmic
	logarithmic := fyne.NewMenuItem("Logarithmic (DPS + 1)", func() {
		ui.graphLogarithmic = true
		ui.saveGraphFormatPreferences()
		ui.renderSelection()
	})
	logarithmic.Checked = ui.graphLogarithmic
	characters := fyne.NewMenuItem("Per character", func() { ui.groupSelect.SetSelected("Characters") })
	characters.Checked = !ui.anyGrouping()
	ships := fyne.NewMenuItem("By ship type (ship history)", func() { ui.groupSelect.SetSelected("Ship history") })
	ships.Checked = ui.shipGrouping() && !ui.shipClassGrouping()
	classes := fyne.NewMenuItem("By ship class (ship history)", func() { ui.groupSelect.SetSelected("Ship class history") })
	classes.Checked = ui.shipClassGrouping()
	groups := fyne.NewMenuItem("By character group", func() { ui.groupSelect.SetSelected(characterGroupsMode) })
	groups.Checked = ui.groupSelect.Selected == characterGroupsMode
	filters := fyne.NewMenuItem("Character group filters", nil)
	all := fyne.NewMenuItem("All", func() { ui.groupSelect.SetSelected(characterGroupsMode) })
	all.Checked = ui.groupSelect.Selected == characterGroupsMode
	groupFilters := []*fyne.MenuItem{all}
	for _, group := range ui.characterGroups {
		option := "Group: " + group.Name
		label := group.Name
		if label == "All" {
			label = "All (group)"
		}
		item := fyne.NewMenuItem(label, func() { ui.groupSelect.SetSelected(option) })
		item.Checked = ui.groupSelect.Selected == option
		groupFilters = append(groupFilters, item)
	}
	filters.ChildMenu = fyne.NewMenu("Character groups", groupFilters...)
	average := fyne.NewMenuItem("10-second DPS average", func() {
		ui.graphAverage10s = !ui.graphAverage10s
		ui.saveGraphFormatPreferences()
		ui.renderSelection()
		ui.rebuildCharacters()
	})
	average.Checked = ui.graphAverage10s
	return fyne.NewMenu("Format options", linear, logarithmic, fyne.NewMenuItemSeparator(), characters, ships, classes, groups, filters, fyne.NewMenuItemSeparator(), average)
}

func (ui *analyzerUI) updateGraphFormatLabel() {
	if ui.graphFormatLabel == nil {
		return
	}
	scale, grouping := "Linear", "Per character"
	if ui.graphLogarithmic {
		scale = "Logarithmic (DPS + 1)"
	}
	if ui.shipGrouping() {
		grouping = "By ship type · history at event time"
	}
	if ui.shipClassGrouping() {
		grouping = "By ship class · history at event time"
	}
	if ui.characterGroupGrouping() {
		grouping = "By character group · All"
		if ui.selectedCharacterGroupID() != "" {
			grouping = ui.groupSelect.Selected
		}
	}
	text := scale + " · " + grouping
	if ui.graphAverage10s {
		text += " · 10-second average"
	}
	ui.graphFormatLabel.SetText(text)
	ui.updateGroupingNote()
}
