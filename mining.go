package main

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const (
	filterOreName           = "Ore Name"
	filterOreType           = "Ore Type"
	filterMiningMessage     = "Message Type"
	miningFiltersPreference = "graph.mining.filters"
)

var miningFilterKinds = []string{filterOreName, filterOreType, filterMiningMessage}
var miningMessageOptions = []damageFilterOption{{Key: miningMined, Label: miningMined}, {Key: miningCompressed, Label: miningCompressed}, {Key: miningWaste, Label: miningWaste}}

func (ui *analyzerUI) initMining() {
	ui.mining = newDamageChart()
	ui.mining.OnRangeSelected, ui.mining.OnWheelZoom = ui.applyGraphRange, ui.applyGraphWheelRange
	if err := json.Unmarshal([]byte(fyne.CurrentApp().Preferences().String(miningFiltersPreference)), &ui.miningFilters); err != nil {
		ui.miningFilters = nil
	}
	if ui.miningFilters == nil {
		ui.miningFilters = map[string]*damageFilterSelection{}
	}
	for _, kind := range miningFilterKinds {
		if ui.miningFilters[kind] == nil {
			ui.miningFilters[kind] = &damageFilterSelection{Default: true}
		}
	}
	ui.miningFilterButtons = map[string]*widget.Button{}
}

func (ui *analyzerUI) miningControls() fyne.CanvasObject {
	return ui.scopedFilterBar(graphMining, ui.eventGroupButton(graphMining))
}

func (ui *analyzerUI) saveMiningFilters() {
	data, _ := json.Marshal(ui.miningFilters)
	fyne.CurrentApp().Preferences().SetString(miningFiltersPreference, string(data))
	ui.renderSelection()
}

func (ui *analyzerUI) refreshMiningControls() {
	for kind, button := range ui.miningFilterButtons {
		options := ui.scopedOptions[graphMining][kind]
		if kind == filterMiningMessage {
			options = miningMessageOptions
		}
		button.SetText(kind + ": " + selectionLabel(ui.miningFilters[kind], options))
	}
}

func (ui *analyzerUI) miningPlot(selected map[string]bool) plotData {
	data := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket, RateUnit: "units/s", HitUnit: "units", IndividualHits: ui.graphAverageWindow == 0,
		EmptyText: "No mining records for the selected ores, message types and characters"}
	builder, ok := newEventPlotBuilder(&data, ui.graphAverageWindow)
	if !ok {
		return data
	}
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.Mining {
			if !ui.scopeFilter(graphMining, filterOreName).selected(event.Ore) || !ui.scopeFilter(graphMining, filterOreType).selected(oreType(event.Ore)) || !ui.scopeFilter(graphMining, filterMiningMessage).selected(event.Kind) {
				continue
			}
			if !builder.includes(event.Second) {
				continue
			}
			kind := oreType(event.Ore)
			if ui.eventGraphGroupings[graphMining] == eventByOreName {
				kind = event.Ore
			}
			names := ui.eventGroupingNames(graphMining, log.Character, kind, event.Second)
			for _, name := range names {
				label := name + " · " + event.Kind
				builder.add(label, "units", plotHit{Second: event.Second, Damage: event.Amount, Character: log.Character, Detail: event.Message})
			}
		}
	}
	data.Series = builder.series()
	return data
}
