package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const (
	ewarEvents           = "Effect events"
	ewarCapacitor        = "Capacitor drained (GJ)"
	ewarMetricPreference = "graph.ewar.metric"
)

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
	metric := container.NewGridWrap(fyne.NewSize(250, ui.ewarMetric.MinSize().Height), ui.ewarMetric)
	return ui.scopedFilterBar(graphIncomingEWar, container.NewHBox(metric, ui.eventGroupButton(graphIncomingEWar)))
}

func (ui *analyzerUI) incomingEWarPlot(selected map[string]bool) plotData {
	data := plotData{Start: ui.view.Start, End: ui.view.End, Bucket: ui.view.Bucket,
		RateUnit: "events/s", HitUnit: "events", EmptyText: "No logged incoming E-War in this range for the selected characters"}
	capacitor := ui.ewarMetric.Selected == ewarCapacitor
	if capacitor {
		data.RateUnit, data.HitUnit = "GJ/s", "GJ"
		data.EmptyText = "No logged incoming capacitor drain in this range for the selected characters"
	}
	data.IndividualHits = ui.graphAverageWindow == 0
	builder, ok := newEventPlotBuilder(&data, ui.graphAverageWindow)
	if !ok {
		return data
	}
	for _, log := range ui.analysisSnapshot().Logs {
		if !selected[log.Character] {
			continue
		}
		for _, event := range log.EWar {
			if capacitor && event.Effect != "Energy neutralizer" && event.Effect != "Nosferatu" {
				continue
			}
			if !builder.includes(event.Second) {
				continue
			}
			for _, name := range ui.eventGroupingNames(graphIncomingEWar, log.Character, event.Effect, event.Second) {
				value := float64(1)
				if capacitor {
					value = event.Amount
				}
				builder.add(name, "", plotHit{Second: event.Second, Damage: value, Character: log.Character, Detail: event.Message})
			}
		}
	}
	data.Series = builder.series()
	return data
}
