package main

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const (
	eventByType      = "Type"
	eventByCharacter = "Character"
	eventByGroup     = "Character Group"
	eventByShip      = "Ship type (ship history)"
	eventByClass     = "Ship class (ship history)"
	eventByOreType   = "Ore Type"
	eventByOreName   = "Ore Name"
	eventByCriticals = "Criticals"
)

var eventGroupingModes = []string{eventByType, eventByCharacter, eventByGroup, eventByShip, eventByClass, filterSystem}

func eventGroupingOptions(id string) []string {
	if id == graphOutgoingLogistics {
		return append(append([]string(nil), eventGroupingModes...), filterTarget, filterTargetType)
	}
	if id == graphMining {
		return []string{eventByCharacter, eventByGroup, eventByShip, eventByClass, filterSystem, eventByOreName, eventByOreType, eventByCriticals}
	}
	return eventGroupingModes
}

func (ui *analyzerUI) initEventGraphGrouping() {
	ui.eventGraphGroupings = map[string]string{}
	for _, id := range []string{graphOutgoingLogistics, graphIncomingLogistics, graphIncomingEWar, graphMining} {
		modes := eventGroupingOptions(id)
		mode := fyne.CurrentApp().Preferences().StringWithFallback("graph.grouping."+id, modes[0])
		if !slices.Contains(modes, mode) {
			mode = modes[0]
		}
		ui.eventGraphGroupings[id] = mode
	}
}

func (ui *analyzerUI) eventGroupingMenu(id string) *fyne.Menu {
	menu := fyne.NewMenu("Group By")
	for _, mode := range eventGroupingOptions(id) {
		grouping := mode
		item := fyne.NewMenuItem(grouping, func() {
			ui.eventGraphGroupings[id] = grouping
			fyne.CurrentApp().Preferences().SetString("graph.grouping."+id, grouping)
			ui.renderSelection()
		})
		item.Checked = ui.eventGraphGroupings[id] == grouping
		menu.Items = append(menu.Items, item)
	}
	return menu
}

func (ui *analyzerUI) eventGroupButton(id string) *widget.Button {
	return ui.graphMenuButton("Group By", func() *fyne.Menu { return ui.eventGroupingMenu(id) })
}

// Group the log character at the event timestamp, before rolling averages.
// Event graphs keep independent grouping choices and never read outgoing's
// groupSelect; overlapping saved groups retain their existing semantics.
func (ui *analyzerUI) eventGroupingNames(id, character, kind string, second int64) []string {
	switch ui.eventGraphGroupings[id] {
	case filterSystem:
		return []string{ui.graphMetadata().attributes(character, second).system.Label}
	case eventByCharacter:
		return []string{character}
	case eventByGroup:
		return ui.hitCharacterGroupsFor(id, character, "")
	case eventByShip, eventByClass:
		attributes := ui.graphMetadata().attributes(character, second)
		hull, class := attributes.hull, attributes.class
		if ui.eventGraphGroupings[id] == eventByClass {
			return []string{class.Label}
		}
		return []string{hull.Label}
	default:
		return []string{kind}
	}
}
