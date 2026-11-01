package main

import (
	"fmt"
	"strings"
	"time"
)

func (ui *analyzerUI) graphInformationText(id string) string {
	parts := []string{ui.graphGroupingDescription(id)}
	if ui.graphAverageWindow > 0 {
		parts = append(parts, fmt.Sprintf("%d-second average", int(ui.graphAverageWindow/time.Second)))
	} else if id == graphIncoming || id == graphOutgoing {
		parts = append(parts, "individual hits")
	} else {
		parts = append(parts, "individual events")
	}
	metric := "damage"
	switch id {
	case graphOutgoingLogistics, graphIncomingLogistics:
		types := ui.scopeFilter(id, filterLogisticsType)
		repairs := types.selected("Shield repair") || types.selected("Armor repair")
		capacitor := types.selected("Capacitor transfer")
		metric = "HP"
		if capacitor && !repairs {
			metric = "GJ"
		} else if capacitor {
			metric = "HP and GJ"
		}
	case graphMining:
		metric = "ore units"
	case graphIncomingEWar:
		metric = "effect events"
		if ui.ewarMetric != nil && ui.ewarMetric.Selected == ewarCapacitor {
			metric = "capacitor drained (GJ)"
		}
	}
	if ui.graphAverageWindow > 0 {
		metric += " per second"
	}
	parts = append(parts, metric)
	var filters []string
	for _, scope := range []string{globalFilterScope, id} {
		for _, kind := range availableFilters(scope) {
			if !ui.visibleFilters[scope][kind] {
				continue
			}
			text := kind + ": " + ui.informationFilterValues(scope, kind)
			if scope == globalFilterScope {
				text += " (global)"
			}
			filters = append(filters, text)
		}
	}
	if len(filters) == 0 {
		parts = append(parts, "Filters: none")
	} else {
		parts = append(parts, "Filters: "+strings.Join(filters, "; "))
	}
	if id == graphOutgoing {
		state := "off"
		if ui.graphInvulnerabilityMarkers {
			state = "on"
		}
		parts = append(parts, "Invulnerability indicators: "+state)
	}
	return strings.Join(parts, " · ")
}

func (ui *analyzerUI) informationFilterValues(scope, kind string) string {
	selection := ui.scopeFilter(scope, kind)
	options := ui.scopeFilterOptions(scope, kind)
	var selected []string
	for _, option := range options {
		checked := selection.selected(option.Key)
		if scope == globalFilterScope && kind == filterCharacters {
			checked = ui.damageFilterSelected(kind, option.Key)
		}
		if checked {
			selected = append(selected, option.Label)
		}
	}
	if len(selected) == 0 && (len(options) > 0 || !selection.selected("")) {
		return "None"
	}
	if len(selected) == len(options) {
		return "All"
	}
	const limit = 3
	if len(selected) > limit {
		return strings.Join(selected[:limit], ", ") + fmt.Sprintf(" (+%d more)", len(selected)-limit)
	}
	return strings.Join(selected, ", ")
}

func (ui *analyzerUI) graphGroupingDescription(id string) string {
	mode := ui.incomingGrouping
	if id == graphOutgoing {
		mode = "Character"
		switch {
		case ui.shipClassGrouping():
			mode = eventByClass
		case ui.shipGrouping():
			mode = eventByShip
		case ui.weaponGrouping(), ui.systemGrouping():
			mode = ui.groupSelect.Selected
		case ui.characterGroupGrouping():
			if ui.selectedCharacterGroupID() != "" {
				return ui.groupSelect.Selected
			}
			mode = eventByGroup
		}
	} else if id != graphIncoming {
		mode = ui.eventGraphGroupings[id]
		if mode == "" {
			mode = eventGroupingOptions(id)[0]
		}
	}
	switch mode {
	case eventByShip:
		return "By ship type · history at event time"
	case eventByClass:
		return "By ship class · history at event time"
	case incomingByShip:
		return "By attacker ship"
	case eventByType:
		if id == graphIncomingEWar {
			return "By effect type"
		}
		return "By assistance type"
	}
	return "By " + strings.ToLower(mode)
}
