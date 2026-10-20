package main

import (
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

const (
	logisticsRepairs   = "Shield & armor reps (HP)"
	logisticsCapacitor = "Capacitor transfers (GJ)"
)

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
	newMetric := func(key string) *widget.Select {
		metric := widget.NewSelect([]string{logisticsRepairs, logisticsCapacitor}, func(value string) {
			fyne.CurrentApp().Preferences().SetString(key, value)
			ui.renderSelection()
		})
		metric.Selected = fyne.CurrentApp().Preferences().StringWithFallback(key, logisticsRepairs)
		if metric.Selected != logisticsCapacitor {
			metric.Selected = logisticsRepairs
		}
		return metric
	}
	ui.outgoingLogisticsMetric = newMetric("graph.logistics.outgoing.metric")
	ui.incomingLogisticsMetric = newMetric("graph.logistics.incoming.metric")
}

func logisticsControls(metric *widget.Select) fyne.CanvasObject {
	note := widget.NewLabel("By assistance type · averages: HP/s or GJ/s · individual transfers: HP or GJ")
	note.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, nil, container.NewGridWrap(fyne.NewSize(265, metric.MinSize().Height), metric), note)
}

func (ui *analyzerUI) logisticsPlot(selected map[string]bool, incoming bool) plotData {
	metric, direction := ui.outgoingLogisticsMetric, "outgoing"
	if incoming {
		metric, direction = ui.incomingLogisticsMetric, "incoming"
	}
	data := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket, RateUnit: "HP/s", HitUnit: "HP", IndividualHits: !ui.graphAverage10s}
	capacitor := metric.Selected == logisticsCapacitor
	if capacitor {
		data.RateUnit, data.HitUnit = "GJ/s", "GJ"
	}
	data.EmptyText = "No logged " + direction + " remote repairs in this range for the selected characters"
	if capacitor {
		data.EmptyText = "No logged " + direction + " capacitor transfers in this range for the selected characters"
	}
	window := int64(1)
	if ui.graphAverage10s {
		data.AverageWindow, window = dpsAverageWindow, int64(dpsAverageWindow/time.Second)
	}
	if !data.End.After(data.Start) || data.Bucket < time.Second {
		return data
	}
	first, end, bucket := data.Start.Unix(), data.End.Unix(), int64(data.Bucket/time.Second)
	n := int(math.Ceil(data.End.Sub(data.Start).Seconds() / data.Bucket.Seconds()))
	groups := map[string]*plotSeries{}
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.Logistics {
			if event.Incoming != incoming || (event.Kind == "Capacitor transfer") != capacitor {
				continue
			}
			left, right := max(first, event.Second), min(end, event.Second+window)
			if left >= right {
				continue
			}
			series := groups[event.Kind]
			if series == nil {
				series = &plotSeries{Name: event.Kind, Color: characterColor(event.Kind), Values: make([]float64, n)}
				groups[event.Kind] = series
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
	for _, series := range groups {
		sort.SliceStable(series.Hits, func(i, j int) bool { return series.Hits[i].Second < series.Hits[j].Second })
		data.Series = append(data.Series, *series)
	}
	sort.Slice(data.Series, func(i, j int) bool { return data.Series[i].Name < data.Series[j].Name })
	return data
}
