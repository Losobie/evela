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
	ewarEvents           = "Effect events"
	ewarCapacitor        = "Capacitor drained (GJ)"
	ewarMetricPreference = "graph.ewar.metric"
)

type ewarEvent struct {
	Second  int64
	Effect  string
	Amount  float64 // GJ lost; only capacitor warfare has a numeric magnitude.
	Message string
}

var (
	capacitorWarfareMessage = regexp.MustCompile(`(?i)^([+-]?[0-9][0-9,.]*)\s+GJ\s+energy\s+(neutralized|neutralised|drained)\s+(.+)$`)
	ewarAttemptMessage      = regexp.MustCompile(`(?i)^(.+?) attempt from (.+) to you[!.]?$`)
	ewarReceivedMessage     = regexp.MustCompile(`(?i)^You (?:are|have been) (?:being )?(.+?)(?: by .+)?[!.]?$`)
)

// Recognize incoming messages, never module names mentioned in damage, chat,
// activation errors or outgoing notifications. A log entry is an event, not
// evidence of effect duration, strength or (for attempts) successful application.
func parseIncomingEWar(message string, second int64) (ewarEvent, bool) {
	event := ewarEvent{Second: second, Message: strings.Clone(message)}
	if match := capacitorWarfareMessage.FindStringSubmatch(message); match != nil {
		amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
			return ewarEvent{}, false
		}
		direction := strings.ToLower(match[3])
		if strings.EqualFold(match[2], "drained") {
			// NOS gains are '+N ... drained from'; losses are '-N ... drained to'.
			if !strings.HasPrefix(direction, "to ") || strings.HasPrefix(match[1], "+") || amount > 0 {
				return ewarEvent{}, false
			}
			event.Effect = "Nosferatu"
		} else {
			// The victim's neutralizer message has an unsigned amount and source.
			if amount < 0 || strings.HasPrefix(match[1], "+") || strings.HasPrefix(direction, "to ") || strings.HasPrefix(direction, "by you") {
				return ewarEvent{}, false
			}
			event.Effect = "Energy neutralizer"
		}
		event.Amount = math.Abs(amount)
		return event, true
	}
	var action string
	if match := ewarAttemptMessage.FindStringSubmatch(message); match != nil {
		action = match[1]
	} else if match := ewarReceivedMessage.FindStringSubmatch(message); match != nil {
		action = match[1]
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "warp scramble", "warp scrambled":
		event.Effect = "Warp scramble"
	case "warp disruption", "warp disrupted":
		event.Effect = "Warp disruption"
	case "stasis webifying", "stasis webification", "webifying", "webified":
		event.Effect = "Stasis webifier"
	case "target painting", "target painted":
		event.Effect = "Target painter"
	case "tracking disruption", "tracking disrupted", "turret disruption", "turret disrupted":
		event.Effect = "Turret disruption"
	case "guidance disruption", "guidance disrupted", "missile disruption", "missile disrupted":
		event.Effect = "Missile disruption"
	case "sensor dampening", "sensor dampened", "remote sensor dampening":
		event.Effect = "Sensor dampening"
	case "sensor jam", "sensor jamming", "electronic counter measures", "jammed":
		event.Effect = "ECM"
	default:
		return ewarEvent{}, false
	}
	return event, true
}

func (ui *analyzerUI) initEWar() {
	ui.ewar = newDamageChart()
	ui.ewar.OnRangeSelected, ui.ewar.OnWheelZoom = ui.applyGraphRange, ui.applyGraphWheelRange
	ui.ewarMetric = widget.NewSelect([]string{ewarEvents, ewarCapacitor}, func(value string) {
		fyne.CurrentApp().Preferences().SetString(ewarMetricPreference, value)
		ui.renderSelection()
	})
	ui.ewarMetric.Selected = fyne.CurrentApp().Preferences().StringWithFallback(ewarMetricPreference, ewarEvents)
	if ui.ewarMetric.Selected != ewarCapacitor {
		ui.ewarMetric.Selected = ewarEvents
	}
}

func (ui *analyzerUI) ewarControls() fyne.CanvasObject {
	note := widget.NewLabel("By effect type · averages: events/s or GJ/s · unlogged effects unavailable")
	note.Truncation = fyne.TextTruncateEllipsis
	metric := container.NewGridWrap(fyne.NewSize(250, ui.ewarMetric.MinSize().Height), ui.ewarMetric)
	return container.NewBorder(nil, nil, nil, metric, note)
}

func (ui *analyzerUI) incomingEWarPlot(selected map[string]bool) plotData {
	data := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket,
		RateUnit: "events/s", HitUnit: "events", EmptyText: "No logged incoming E-War in this range for the selected characters"}
	capacitor := ui.ewarMetric.Selected == ewarCapacitor
	if capacitor {
		data.RateUnit, data.HitUnit = "GJ/s", "GJ"
		data.EmptyText = "No logged incoming capacitor drain in this range for the selected characters"
	}
	data.IndividualHits = !ui.graphAverage10s
	window := int64(1)
	if ui.graphAverage10s {
		data.AverageWindow = dpsAverageWindow
		window = int64(dpsAverageWindow / time.Second)
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
		for _, event := range log.EWar {
			if capacitor && event.Effect != "Energy neutralizer" && event.Effect != "Nosferatu" {
				continue
			}
			left, right := max(first, event.Second), min(end, event.Second+window)
			if left >= right {
				continue
			}
			series := groups[event.Effect]
			if series == nil {
				series = &plotSeries{Name: event.Effect, Color: characterColor(event.Effect), Values: make([]float64, n)}
				groups[event.Effect] = series
			}
			value := float64(1)
			if capacitor {
				value = event.Amount
			}
			if event.Second >= first {
				series.Hits = append(series.Hits, plotHit{Second: event.Second, Damage: value, Character: log.Character, Detail: event.Message})
			}
			for bin := int((left - first) / bucket); bin <= int((right-1-first)/bucket); bin++ {
				binStart, binEnd := first+int64(bin)*bucket, min(end, first+int64(bin+1)*bucket)
				overlap := min(right, binEnd) - max(left, binStart)
				series.Values[bin] += value * float64(overlap) / float64(window) / float64(binEnd-binStart)
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
