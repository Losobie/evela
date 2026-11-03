package main

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// Exercise coarse/partial-bucket arithmetic directly; the UI always uses Auto.
func setTestGraphBucket(ui *analyzerUI, bucket time.Duration) {
	ui.view = buildView(ui.analysisSnapshot(), ui.view.Start, ui.view.End, bucket)
	ui.renderSelection()
}

func TestThirtySecondAverageSharedAcrossGraphsAndGroupingModes(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("Thirty second average")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 27, 2, 0, 0, 0, time.UTC)
	log := &parsedLog{Character: "Miner"}
	for _, row := range []struct {
		second int64
		amount float64
	}{{start.Unix() - 29, 900}, {start.Unix(), 300}} {
		log.Points = append(log.Points, damagePoint{Second: row.second, Outgoing: row.amount, Incoming: row.amount * 2, OutgoingHits: 1, IncomingHits: 1})
		log.Logistics = append(log.Logistics, logisticsEvent{Second: row.second, Kind: "Armor repair", Amount: row.amount}, logisticsEvent{Second: row.second, Kind: "Armor repair", Amount: row.amount * 2, Incoming: true})
		log.Mining = append(log.Mining, miningEvent{Second: row.second, Kind: miningMined, Ore: "Gneiss", Amount: row.amount})
		log.EWar = append(log.EWar, ewarEvent{Second: row.second, Effect: "Energy neutralizer", Amount: row.amount})
	}
	ui.snapshot = &logSnapshot{Characters: []string{log.Character}, Logs: []*parsedLog{log}, First: start.Add(-29 * time.Second), Last: start}
	ui.selected[log.Character] = true
	ui.ewarMetric.SetSelected(ewarCapacitor)
	ui.applyGraphRange(start, start.Add(33*time.Second))
	applyTestShipRecords(ui, []shipRecord{{CharacterName: log.Character, TypeID: 1, TypeName: "Hulk", ClassName: "Exhumer", CheckedAt: time.Now()}})
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	totals, summary, history := append([]characterSeries(nil), ui.tableSeries...), ui.summary.Text, len(ui.graphUndo)
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "30-second DPS average")
	charts := []*damageChart{ui.outgoing, ui.incoming, ui.outgoingLogistics, ui.incomingLogistics, ui.ewar, ui.mining}
	for index, chart := range charts {
		if chart.data.AverageWindow != 30*time.Second || chart.data.IndividualHits {
			t.Fatalf("graph %d did not use thirty seconds", index)
		}
		series := chart.data.Series[0]
		factor := 1.0
		if chart == ui.incoming || chart == ui.incomingLogistics {
			factor = 2
		}
		if math.Abs(series.Values[0]-40*factor) > 1e-9 || math.Abs(series.Values[1]-10*factor) > 1e-9 || series.Values[30] != 0 {
			t.Fatalf("graph %d rolling rates: %v", index, series.Values)
		}
	}
	if !reflect.DeepEqual(totals, ui.tableSeries) || summary != ui.summary.Text || len(ui.graphUndo) != history {
		t.Fatal("average changed raw totals or navigation history")
	}
	for _, item := range ui.graphOptionsMenu().Items {
		if item.Label == "10-second DPS average" && item.Checked || item.Label == "30-second DPS average" && !item.Checked {
			t.Fatal("average choices are not mutually exclusive")
		}
	}
	// Reuse the same view while changing windows to exercise cached averages.
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "10-second DPS average")
	if ui.outgoing.data.Series[0].Values[0] != 30 {
		t.Fatal("switching back to ten seconds used a stale cache")
	}
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "30-second DPS average")
	if ui.outgoing.data.Series[0].Values[0] != 40 {
		t.Fatal("switching to thirty seconds used a stale cache")
	}
	chooseGraphMenuItem(t, ui.graphFormatMenu(), "Ship type (ship history)")
	if ui.outgoing.data.Series[0].Name != "Hulk" || ui.outgoing.data.Series[0].Values[0] != 40 {
		t.Fatal("historical ship grouping lost the average window")
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByAttacker)
	if ui.incoming.data.Series[0].Values[0] != 80 {
		t.Fatal("incoming source grouping lost the average window")
	}
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft, 60, 0))
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, "(30s avg)") {
		t.Fatalf("hover average window: %q", ui.outgoing.hoverReadings[0].Text)
	}
	// Coarse and partial buckets retain the per-second denominator and lookback.
	setTestGraphBucket(ui, 5*time.Second)
	values := ui.outgoing.data.Series[0].Values
	if math.Abs(values[0]-16) > 1e-9 || math.Abs(values[len(values)-1]) > 1e-9 {
		t.Fatalf("coarse or partial thirty-second buckets: %v", values)
	}
	other := app.NewWindow("Restored average")
	defer other.Close()
	restored := newAnalyzerUI(other)
	if restored.graphAverageWindow != 30*time.Second || restored.outgoing.data.AverageWindow != 30*time.Second {
		t.Fatal("thirty-second average was not restored")
	}
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "30-second DPS average")
	if ui.graphAverageWindow != 0 || !ui.outgoing.data.IndividualHits || !ui.mining.data.IndividualHits {
		t.Fatal("toggling the selected average did not restore individual events")
	}
}

func TestAverageWindowPreferencesMigrateAndValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		legacy  bool
		seconds int
		want    time.Duration
	}{
		{"legacy enabled", true, -1, 10 * time.Second},
		{"legacy disabled", false, -1, 0},
		{"saved thirty", false, 30, 30 * time.Second},
		{"saved ten", false, 10, 10 * time.Second},
		{"saved disabled", true, 0, 0},
		{"invalid enabled", true, 17, 10 * time.Second},
		{"invalid disabled", false, 17, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := test.NewTempApp(t)
			app.Preferences().SetBool(graphAveragePreference, tc.legacy)
			if tc.seconds >= 0 {
				app.Preferences().SetInt(graphAverageSecondsPreference, tc.seconds)
			}
			window := app.NewWindow("Average migration")
			defer window.Close()
			ui := newAnalyzerUI(window)
			if ui.graphAverageWindow != tc.want {
				t.Fatalf("average=%v, want %v", ui.graphAverageWindow, tc.want)
			}
		})
	}
}
