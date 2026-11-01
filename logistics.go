package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var logisticsTypeOptions = []damageFilterOption{{Key: "Shield repair", Label: "Shield"}, {Key: "Armor repair", Label: "Armor"}, {Key: "Capacitor transfer", Label: "Capacitor"}}

type logisticsEvent struct {
	Second   int64
	Kind     string
	Amount   float64
	Incoming bool
	Message  string
}

// Incoming remote assistance uses "by" (also accept explicit "from");
// outgoing assistance uses "to". Require a remote-assistance verb so local
// tanking, capacitor boosters, NOS and neutralizers cannot become logistics.
var logisticsMessage = regexp.MustCompile(`(?i)^([0-9][0-9,.]*)(?:\s+(?:HP|GJ))?\s+remote\s+(armor repaired|shield boosted|capacitor transmitted)\s+(to|by|from)\s+(\S.*)$`)

func parseLogistics(message string, second int64) (logisticsEvent, bool) {
	match := logisticsMessage.FindStringSubmatch(message)
	if match == nil {
		return logisticsEvent{}, false
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return logisticsEvent{}, false
	}
	kind := "Capacitor transfer"
	switch strings.ToLower(match[2]) {
	case "armor repaired":
		kind = "Armor repair"
	case "shield boosted":
		kind = "Shield repair"
	}
	return logisticsEvent{Second: second, Kind: kind, Amount: amount, Incoming: !strings.EqualFold(match[3], "to"), Message: strings.Clone(message)}, true
}

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
	note := widget.NewLabel("Repairs: HP · Capacitor: GJ · averaged rates per second")
	note.Truncation = fyne.TextTruncateEllipsis
	return container.NewVBox(container.NewBorder(nil, nil, nil, ui.eventGroupButton(id), note), ui.scopedFilterBar(id))
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
		count := 0
		for _, option := range logisticsTypeOptions {
			if ui.logisticsTypes[id].selected(option.Key) {
				count++
			}
		}
		label := "All"
		if count == 0 {
			label = "None"
		} else if count < len(logisticsTypeOptions) {
			label = fmt.Sprintf("%d/%d", count, len(logisticsTypeOptions))
		}
		button.SetText("Type: " + label)
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
	window := int64(1)
	if ui.graphAverageWindow > 0 {
		data.AverageWindow, window = ui.graphAverageWindow, int64(ui.graphAverageWindow/time.Second)
	}
	if !data.End.After(data.Start) || data.Bucket < time.Second {
		return data
	}
	first, end, bucket := data.Start.Unix(), data.End.Unix(), int64(data.Bucket/time.Second)
	n := int(math.Ceil(data.End.Sub(data.Start).Seconds() / data.Bucket.Seconds()))
	groups := map[string]*plotSeries{}
	ships := ui.shipHistory.indexByName()
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.Logistics {
			if event.Incoming != incoming || !types.selected(event.Kind) {
				continue
			}
			left, right := max(first, event.Second), min(end, event.Second+window)
			if left >= right {
				continue
			}
			unit := "HP"
			if event.Kind == "Capacitor transfer" {
				unit = "GJ"
			}
			for _, name := range ui.eventGroupingNames(id, log.Character, event.Kind, event.Second, ships) {
				key := name + "\x00" + unit
				label := name
				if ui.eventGraphGroupings[id] != eventByType && data.SecondaryUnit != "" {
					label += " · " + unit
				}
				series := groups[key]
				if series == nil {
					series = &plotSeries{Name: label, Unit: unit, Color: characterColor(label), Values: make([]float64, n)}
					groups[key] = series
				}
				if event.Second >= first {
					series.Hits = append(series.Hits, plotHit{Second: event.Second, Damage: event.Amount, Character: log.Character, Detail: event.Message})
				}
				for bin := int((left - first) / bucket); bin <= int((right-1-first)/bucket); bin++ {
					binStart, binEnd := first+int64(bin)*bucket, min(end, first+int64(bin+1)*bucket)
					overlap := min(right, binEnd) - max(left, binStart)
					series.Values[bin] += event.Amount * float64(overlap) / float64(window) / float64(binEnd-binStart)
				}
			}
		}
	}
	for _, series := range groups {
		sort.SliceStable(series.Hits, func(i, j int) bool { return series.Hits[i].Second < series.Hits[j].Second })
		data.Series = append(data.Series, *series)
	}
	sort.Slice(data.Series, func(i, j int) bool { return data.Series[i].Name < data.Series[j].Name })
	return data
}
