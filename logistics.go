package main

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

var logisticsTypeOptions = []damageFilterOption{{Key: "Shield repair", Label: "Shield"}, {Key: "Armor repair", Label: "Armor"}, {Key: "Capacitor transfer", Label: "Capacitor"}}

func (ui *analyzerUI) initLogistics() {
	ui.outgoingLogistics, ui.incomingLogistics = newDamageChart(), newDamageChart()
	for _, chart := range []*damageChart{ui.outgoingLogistics, ui.incomingLogistics} {
		chart.OnRangeSelected, chart.OnWheelZoom = ui.applyGraphRange, ui.applyGraphWheelRange
	}
	ui.logisticsTypes = map[string]*damageFilterSelection{}
	ui.logisticsTypeButtons = map[string]*widget.Button{}
	for _, id := range []string{graphOutgoingLogistics, graphIncomingLogistics} {
		selection := &damageFilterSelection{Default: true}
		saved := fyne.CurrentApp().Preferences().String("graph.logistics.types." + id)
		if json.Unmarshal([]byte(saved), selection) != nil {
			selection = &damageFilterSelection{Default: true}
			// Migrate the old exclusive metric without resetting an explicit choice.
			direction := "outgoing"
			if id == graphIncomingLogistics {
				direction = "incoming"
			}
			switch fyne.CurrentApp().Preferences().String("graph.logistics." + direction + ".metric") {
			case "Shield & armor reps (HP)":
				selection.set("Capacitor transfer", false)
			case "Capacitor transfers (GJ)":
				selection.Default = false
				selection.set("Capacitor transfer", true)
			}
		}
		ui.logisticsTypes[id] = selection
	}
}

func (ui *analyzerUI) logisticsControls(id string) fyne.CanvasObject {
	return ui.scopedFilterBar(id, ui.eventGroupButton(id))
}

func (ui *analyzerUI) saveLogisticsTypes(id string) {
	ui.visibleFilters[id][filterLogisticsType] = true
	ui.saveScopedFilters()
	ui.refreshScopedFilterBars()
	data, _ := json.Marshal(ui.logisticsTypes[id])
	fyne.CurrentApp().Preferences().SetString("graph.logistics.types."+id, string(data))
	ui.renderSelection()
}

func (ui *analyzerUI) setLogisticsType(id, key string, checked bool) {
	ui.logisticsTypes[id].set(key, checked)
	ui.saveLogisticsTypes(id)
}

func (ui *analyzerUI) setAllLogisticsTypes(id string, checked bool) {
	ui.logisticsTypes[id].Default, ui.logisticsTypes[id].Values = checked, map[string]bool{}
	ui.saveLogisticsTypes(id)
}

func (ui *analyzerUI) refreshLogisticsTypeControls() {
	for id, button := range ui.logisticsTypeButtons {
		button.SetText("Type: " + selectionLabel(ui.logisticsTypes[id], logisticsTypeOptions))
	}
}

func (ui *analyzerUI) logisticsPlot(selected map[string]bool, incoming bool) plotData {
	id, direction := graphOutgoingLogistics, "outgoing"
	if incoming {
		id, direction = graphIncomingLogistics, "incoming"
	}
	data := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket, RateUnit: "HP/s", HitUnit: "HP", IndividualHits: ui.graphAverageWindow == 0}
	types := ui.scopeFilter(id, filterLogisticsType)
	repairs, capacitor := types.selected("Shield repair") || types.selected("Armor repair"), types.selected("Capacitor transfer")
	if capacitor && !repairs {
		data.RateUnit, data.HitUnit = "GJ/s", "GJ"
	}
	if capacitor && repairs {
		data.SecondaryUnit = "GJ"
	}
	data.EmptyText = "No logged " + direction + " logistics for the selected types and characters"
	builder, ok := newEventPlotBuilder(&data, ui.graphAverageWindow)
	if !ok {
		return data
	}
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.Logistics {
			if event.Incoming != incoming || !types.selected(event.Kind) {
				continue
			}
			if !builder.includes(event.Second) {
				continue
			}
			unit := "HP"
			if event.Kind == "Capacitor transfer" {
				unit = "GJ"
			}
			names := ui.eventGroupingNames(id, log.Character, event.Kind, event.Second)
			if mode := ui.eventGraphGroupings[id]; targetGroupingMode(mode) {
				names = []string{namedDamageOption(targetValue(event.Target, event.TargetType, mode), mode).Label}
			}
			for _, name := range names {
				label := name
				if ui.eventGraphGroupings[id] != eventByType && data.SecondaryUnit != "" {
					label += " · " + unit
				}
				builder.add(label, unit, plotHit{Second: event.Second, Damage: event.Amount, Character: log.Character, Detail: event.Message})
			}
		}
	}
	data.Series = builder.series()
	return data
}
