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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	filterOreName           = "Ore Name"
	filterOreType           = "Ore Type"
	filterMiningMessage     = "Message Type"
	miningMined             = "Ore mined"
	miningCompressed        = "Compressed"
	miningWaste             = "Waste"
	miningFiltersPreference = "graph.mining.filters"
)

var miningFilterKinds = []string{filterOreName, filterOreType, filterMiningMessage}
var miningMessageOptions = []damageFilterOption{{Key: miningMined, Label: miningMined}, {Key: miningCompressed, Label: miningCompressed}, {Key: miningWaste, Label: miningWaste}}
var minedMessage = regexp.MustCompile(`^You mined ([0-9][0-9,]*) units of (.+?)(?: with a lost residue of ([0-9][0-9,]*) units)?\.?$`)
var compressedMessage = regexp.MustCompile(`^Successfully compressed (.+?) into ([0-9][0-9,]*) (.+)\.$`)

type miningEvent struct {
	Second    int64
	Ore, Kind string
	Amount    float64
	Message   string
}

func miningAmount(text string) (float64, bool) {
	// Parse integers first so malformed separators and overflowing quantities
	// cannot silently become misleading graph values.
	if strings.Contains(text, ",") {
		parts := strings.Split(text, ",")
		if len(parts[0]) < 1 || len(parts[0]) > 3 {
			return 0, false
		}
		for _, part := range parts[1:] {
			if len(part) != 3 {
				return 0, false
			}
		}
	}
	amount, err := strconv.ParseUint(strings.ReplaceAll(text, ",", ""), 10, 64)
	return float64(amount), err == nil
}

func parseMining(message, channel string, second int64) []miningEvent {
	if channel == "(mining)" {
		match := minedMessage.FindStringSubmatch(message)
		if match == nil {
			return nil
		}
		amount, ok := miningAmount(match[1])
		if !ok {
			return nil
		}
		ore := strings.TrimSpace(match[2])
		if ore == "" || strings.Contains(ore, " with a lost residue") {
			return nil
		}
		events := []miningEvent{{Second: second, Ore: strings.Clone(ore), Kind: miningMined, Amount: amount, Message: strings.Clone(message)}}
		if match[3] != "" {
			waste, ok := miningAmount(match[3])
			if !ok {
				return nil
			}
			events = append(events, miningEvent{Second: second, Ore: events[0].Ore, Kind: miningWaste, Amount: waste, Message: events[0].Message})
		}
		return events
	}
	if channel == "(notify)" {
		match := compressedMessage.FindStringSubmatch(message)
		if match == nil {
			return nil
		}
		amount, ok := miningAmount(match[2])
		ore := strings.TrimSpace(match[1])
		if !ok || ore == "" || !strings.EqualFold(strings.TrimSpace(match[3]), "Compressed "+ore) {
			return nil
		}
		return []miningEvent{{Second: second, Ore: strings.Clone(ore), Kind: miningCompressed, Amount: amount, Message: strings.Clone(message)}}
	}
	return nil
}

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
	buttons := container.NewHBox()
	for _, kind := range miningFilterKinds {
		category := kind
		var button *widget.Button
		button = widget.NewButtonWithIcon(category+": All", theme.MenuDropDownIcon(), func() {
			ui.openSelectionFilter(category, button, func() []damageFilterOption { return ui.miningFilterOptions[category] },
				func(key string) bool { return ui.miningFilters[category].selected(key) },
				func(key string, checked bool) { ui.miningFilters[category].set(key, checked); ui.saveMiningFilters() },
				func(checked bool) {
					ui.miningFilters[category].Default, ui.miningFilters[category].Values = checked, map[string]bool{}
					ui.saveMiningFilters()
				})
		})
		ui.miningFilterButtons[category] = button
		buttons.Add(button)
	}
	buttons.Add(ui.eventGroupButton(graphMining))
	note := widget.NewLabel("Ore units · mined, compressed and waste shown separately")
	note.Truncation = fyne.TextTruncateEllipsis
	ui.refreshMiningControls()
	return container.NewVBox(note, container.NewBorder(nil, nil, nil, buttons))
}

func (ui *analyzerUI) saveMiningFilters() {
	data, _ := json.Marshal(ui.miningFilters)
	fyne.CurrentApp().Preferences().SetString(miningFiltersPreference, string(data))
	ui.renderSelection()
}

func (ui *analyzerUI) refreshMiningControls() {
	options := map[string]map[string]bool{filterOreName: {}, filterOreType: {}}
	first, end := ui.view.Start.Unix(), ui.view.End.Unix()
	for _, log := range ui.snapshot.Logs {
		for _, event := range log.Mining {
			if event.Second >= first && event.Second < end {
				options[filterOreName][event.Ore] = true
				options[filterOreType][oreType(event.Ore)] = true
			}
		}
	}
	ui.miningFilterOptions = map[string][]damageFilterOption{filterMiningMessage: miningMessageOptions}
	for kind, values := range options {
		for value := range values {
			ui.miningFilterOptions[kind] = append(ui.miningFilterOptions[kind], damageFilterOption{Key: value, Label: value})
		}
		sort.Slice(ui.miningFilterOptions[kind], func(i, j int) bool {
			return compareTotalsText(ui.miningFilterOptions[kind][i].Label, ui.miningFilterOptions[kind][j].Label) < 0
		})
	}
	for kind, button := range ui.miningFilterButtons {
		count := 0
		for _, option := range ui.miningFilterOptions[kind] {
			if ui.miningFilters[kind].selected(option.Key) {
				count++
			}
		}
		label := "All"
		if count == 0 && (len(ui.miningFilterOptions[kind]) > 0 || !ui.miningFilters[kind].Default) {
			label = "None"
		} else if count < len(ui.miningFilterOptions[kind]) {
			label = fmt.Sprintf("%d/%d", count, len(ui.miningFilterOptions[kind]))
		}
		button.SetText(kind + ": " + label)
	}
}

func (ui *analyzerUI) miningPlot(selected map[string]bool) plotData {
	data := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket, RateUnit: "units/s", HitUnit: "units", IndividualHits: ui.graphAverageWindow == 0,
		EmptyText: "No mining records for the selected ores, message types and characters"}
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
		for _, event := range log.Mining {
			if !ui.miningFilters[filterOreName].selected(event.Ore) || !ui.miningFilters[filterOreType].selected(oreType(event.Ore)) || !ui.miningFilters[filterMiningMessage].selected(event.Kind) {
				continue
			}
			left, right := max(first, event.Second), min(end, event.Second+window)
			if left >= right {
				continue
			}
			names := ui.eventGroupingNames(graphMining, log.Character, oreType(event.Ore), event.Second, ships)
			for _, name := range names {
				label := name + " · " + event.Kind
				series := groups[label]
				if series == nil {
					series = &plotSeries{Name: label, Unit: "units", Color: characterColor(label), Values: make([]float64, n)}
					groups[label] = series
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
