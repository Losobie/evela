package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) updateRange() {
	if ui.rangePreset != "Custom" && ui.rangePreset != selectDayOption {
		ui.rangeStart, ui.rangeEnd = presetRange(ui.rangePreset, ui.snapshot)
	}
	ui.rebuildView()
}

func (ui *analyzerUI) rebuildView() { ui.metadata = nil; ui.rebuildViewWithOptions(true) }

// Filter selections do not change the available options in the raw time range.
func (ui *analyzerUI) rebuildFilteredView() { ui.rebuildViewWithOptions(false) }

func (ui *analyzerUI) rebuildViewWithOptions(refreshOptions bool) {
	if ui.totals == nil {
		return
	}
	start, end := ui.rangeStart, ui.rangeEnd
	if !end.After(start) {
		return
	}
	ui.updateRangeLabel()
	if ui.metadata != nil && ui.metadata.snapshot != ui.snapshot {
		ui.metadata = nil
		refreshOptions = true
	}
	refreshOptions = refreshOptions || ui.filterOptions == nil || !start.Equal(ui.view.Start) || !end.Equal(ui.view.End)
	if ui.metadata == nil {
		ui.locationLookup = ui.resolvedLocations()
		ui.migrateSystemFilterNames(ui.locationLookup)
	}
	if refreshOptions {
		ui.graphMetadata().resetTimes()
	}
	// Retain the longest supported average's pre-range tail even while viewing
	// individual hits, so switching averaging on never requires an archive scan.
	input := snapshotInRange(ui.snapshot, start, end, max(30*time.Second, ui.graphAverageWindow))
	ui.analysis = input
	if filtered := ui.filteredDamageSnapshot(input); filtered != nil {
		ui.analysis = filtered
	}
	ui.view = buildView(ui.analysisSnapshot(), start, end, 0)
	if refreshOptions {
		ui.refreshDamageFilterOptions()
	}
	ui.saveGraphRangePreference()
	ui.renderSelection()
	ui.rebuildCharacters()
}

func (ui *analyzerUI) rebuildCharacters() {
	var rows []fyne.CanvasObject
	active := ui.view.activeSeries()
	query := strings.ToLower(strings.TrimSpace(ui.characterSearch.Text))
	if ui.anyGrouping() {
		rows = ui.groupedCharacterList(query)
	} else {
		for _, series := range active {
			name := series.Name
			if !strings.Contains(strings.ToLower(name+" "+shipLabel(name, ui.ships)), query) {
				continue
			}
			rows = append(rows, ui.characterRow(name, "", shipLabel(name, ui.ships)))
		}
	}
	if len(rows) == 0 {
		message := "No matching characters"
		if len(ui.snapshot.Characters) == 0 {
			message = "Characters appear after import"
		} else if len(active) == 0 {
			message = "No damage data in this time range"
		}
		empty := widget.NewLabel(message)
		empty.Wrapping = fyne.TextWrapWord
		rows = append(rows, empty)
	}
	// Swap the complete list in one step. Emptying a scrolled list first would
	// temporarily shrink its content and reset the user's scroll position.
	ui.characters.Objects = rows
	ui.characters.Refresh()
	if ui.characterGroupEditor != nil {
		ui.characterGroupEditor.refreshLists()
	}
}

func (ui *analyzerUI) selectRangeCharacters(selected bool) {
	for _, series := range ui.view.activeSeries() {
		if ui.characterMatchesGroupFilter(series.Name) {
			ui.selected[series.Name] = selected
		}
	}
	ui.syncCharacterFilter()
	ui.rebuildCharacters()
	ui.renderSelection()
}

func (ui *analyzerUI) renderSelection() {
	in := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket}
	if ui.graphAverageWindow > 0 {
		in.AverageWindow = ui.graphAverageWindow
	}
	out := in
	selected := ui.filteredCharacterSelection()
	if ui.graphInvulnerabilityMarkers {
		out.Markers = ui.view.structureMarkers(ui.analysisSnapshot(), selected)
	}
	var incoming, outgoing float64
	var hitsIn, hitsOut int
	contributing := map[string]bool{}
	active := ui.view.activeSeries()
	for _, series := range active {
		if !selected[series.Name] {
			continue
		}
		contributing[series.Name] = true
		incoming += series.TotalIncoming
		outgoing += series.TotalOutgoing
		hitsIn += series.IncomingHits
		hitsOut += series.OutgoingHits
	}
	var filtered []characterSeries
	for _, series := range active {
		if selected[series.Name] {
			filtered = append(filtered, series)
		}
	}
	ui.tableSeries = append([]characterSeries(nil), filtered...)
	graphSeries := active
	if ui.graphAverageWindow > 0 && !ui.shipGrouping() {
		graphSeries = ui.view.rollingAverage(ui.analysisSnapshot(), ui.graphAverageWindow)
	}
	if ui.systemGrouping() {
		ui.tableSeries = ui.view.historicalAttributeSeries(ui.analysisSnapshot(), ui.shipHistory, ui.locationLookup, selected, filterSystem, 0, ui.graphMetadata())
		graphSeries = ui.view.historicalAttributeSeries(ui.analysisSnapshot(), ui.shipHistory, ui.locationLookup, selected, filterSystem, ui.graphAverageWindow, ui.graphMetadata())
	} else if ui.damageAttributeGrouping() {
		ui.tableSeries = ui.view.damageAttributeSeries(ui.analysisSnapshot(), selected, ui.groupSelect.Selected, 0)
		graphSeries = ui.view.damageAttributeSeries(ui.analysisSnapshot(), selected, ui.groupSelect.Selected, ui.graphAverageWindow)
	} else if ui.shipGrouping() {
		ui.tableSeries = ui.groupedShipSeries(selected, 0)
		graphSeries = ui.groupedShipSeries(ui.selected, ui.graphAverageWindow)
	} else if ui.characterGroupGrouping() {
		ui.tableSeries = ui.groupCustomCharacterSeries(active, selected)
		graphSeries = ui.groupCustomCharacterSeries(graphSeries, selected)
	}
	for _, series := range graphSeries {
		if !series.hasActivity(false) || !ui.anyGrouping() && !selected[series.Name] {
			continue
		}
		ink := characterColor(series.Name)
		out.Series = append(out.Series, plotSeries{Name: series.Name, Color: ink, Values: series.Outgoing, MemberCount: len(series.Members), MemberCounts: series.MemberCounts})
	}
	for _, series := range ui.incomingGraphSeries(selected) {
		if !series.hasActivity(true) {
			continue
		}
		in.Series = append(in.Series, plotSeries{Name: series.Name, Color: characterColor(series.Name), Values: series.Incoming, MemberCount: len(series.Members), MemberCounts: series.MemberCounts})
	}
	if ui.graphAverageWindow == 0 {
		ui.attachIndividualHits(&in, &out, selected)
	}
	ui.incoming.setData(in)
	ui.outgoing.setData(out)
	ui.ewar.setData(ui.incomingEWarPlot(selected))
	ui.outgoingLogistics.setData(ui.logisticsPlot(selected, false))
	ui.incomingLogistics.setData(ui.logisticsPlot(selected, true))
	ui.mining.setData(ui.miningPlot(selected))
	ui.sortDamageTotals()
	ui.totals.Refresh()
	var ewarCount int
	var drained float64
	var mined float64
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.EWar {
			if event.Second >= ui.view.Start.Unix() && event.Second < ui.view.End.Unix() {
				contributing[log.Character] = true
				ewarCount++
				drained += event.Amount
			}
		}
		for _, event := range log.Logistics {
			if event.Second >= ui.view.Start.Unix() && event.Second < ui.view.End.Unix() {
				contributing[log.Character] = true
			}
		}
		for _, event := range log.Mining {
			if event.Second >= ui.view.Start.Unix() && event.Second < ui.view.End.Unix() {
				contributing[log.Character] = true
				if event.Kind == miningMined && ui.scopeFilter(graphMining, filterOreName).selected(event.Ore) && ui.scopeFilter(graphMining, filterOreType).selected(oreType(event.Ore)) && ui.scopeFilter(graphMining, filterMiningMessage).selected(event.Kind) {
					mined += event.Amount
				}
			}
		}
	}
	summary := fmt.Sprintf("%d selected   |   Outgoing %s damage / %d hits   |   Incoming %s damage / %d hits", len(contributing), shortNumber(outgoing), hitsOut, shortNumber(incoming), hitsIn)
	if ewarCount > 0 {
		summary += fmt.Sprintf("   |   Incoming E-War %d events / %s GJ", ewarCount, shortNumber(drained))
	}
	if mined > 0 {
		summary += fmt.Sprintf("   |   Mining %s units", shortNumber(mined))
	}
	ui.summary.SetText(summary)
	ui.miningTotal.SetText(fmt.Sprintf("Mining total: %.0f units", mined))
	ui.refreshDamageFilterControls()
	ui.refreshLogisticsTypeControls()
	ui.refreshMiningControls()
	ui.refreshEvents()
	ui.refreshEngagements()
}
