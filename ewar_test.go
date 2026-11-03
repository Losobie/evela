package main

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestIncomingEWarMessageDirectionAndEffects(t *testing.T) {
	for _, tc := range []struct {
		message, effect string
		amount          float64
	}{
		// Published victim-side log formats: unsigned neutralization, negative
		// NOS drained to the aggressor, and incoming warp scramble attempts.
		{"438 GJ energy neutralized Starving Leshak - Starving Leshak", "Energy neutralizer", 438},
		{"-92 GJ energy drained to Bhaalgorn [V P H] [Merhers Republic Mernher] - - Heavy Knave Scoped Energy Nosferatu", "Nosferatu", 92},
		{"Warp scramble attempt from Enemy[CORP](Lachesis) to you!", "Warp scramble", 0},
		{"Warp disruption attempt from Enemy to You!", "Warp disruption", 0},
		// Conditional notifications: parse only if the log actually contains one.
		{"You are being webified by Enemy.", "Stasis webifier", 0},
		{"You have been target painted by Enemy!", "Target painter", 0},
		{"Tracking disruption attempt from Enemy to you!", "Turret disruption", 0},
		{"Guidance disruption attempt from Enemy to you!", "Missile disruption", 0},
		{"Sensor dampening attempt from Enemy to you!", "Sensor dampening", 0},
		{"You are jammed by Enemy.", "ECM", 0},
		{"0 GJ energy neutralized Enemy - Neutralizer", "Energy neutralizer", 0},
		{"-0 GJ energy drained to Enemy - Nosferatu", "Nosferatu", 0},
		{"1,234.5 GJ energy neutralized Enemy - Neutralizer", "Energy neutralizer", 1234.5},
		{"+92 GJ energy drained from Enemy - Nosferatu", "", 0},
		{"92 GJ energy drained from Enemy - Nosferatu", "", 0},
		{"92 GJ energy neutralized to Enemy - Neutralizer", "", 0},
		{"Warp scramble attempt from you to Enemy!", "", 0},
		{"Target painting attempt from Ally to Enemy!", "", 0},
		{"Your Stasis Webifier II cannot be activated.", "", 0},
		{"Your attempt to jam Enemy failed.", "", 0},
		{"You are no longer jammed.", "", 0},
		{"20 from Target Painter - Missile - Hits", "", 0},
		{"20 GJ energy transferred from Ally", "", 0},
		{"20 armor repaired by Ally", "", 0},
	} {
		t.Run(tc.message, func(t *testing.T) {
			event, ok := parseIncomingEWar(tc.message, 100)
			if ok != (tc.effect != "") || event.Effect != tc.effect || event.Amount != tc.amount {
				t.Fatalf("event=%+v accepted=%v, want %q/%g", event, ok, tc.effect, tc.amount)
			}
		})
	}
}

func parseEWarTestLog(t *testing.T) *parsedLog {
	t.Helper()
	log, err := parseLog(strings.NewReader("Listener: Pilot A\n" +
		"[ 2026.10.20 02:00:01 ] (combat) -20 GJ energy drained to Enemy - Nosferatu\n" +
		"[ 2026.10.20 01:59:59 ] (combat) <color=red>100 GJ</color> energy neutralized Enemy - Neutralizer\n" +
		"[ 2026.10.20 02:00:00 ] (combat) 50 GJ energy neutralized Enemy - Neutralizer\n" +
		"[ 2026.10.20 02:00:00 ] (combat) 0 GJ energy neutralized Enemy - Neutralizer\n" +
		"[ 2026.10.20 02:00:02 ] (notify) Warp scramble attempt from Enemy to you!\n" +
		"[ 2026.10.20 02:00:03 ] (chat) You are jammed by Enemy.\n" +
		"[ 2026.10.20 02:00:04 ] (combat) +20 GJ energy drained from Enemy - Nosferatu\n" +
		"[ 2026.10.20 02:00:05 ] (combat) 7 from Enemy - Missile - Hits\n"))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func newEWarTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	application := test.NewTempApp(t)
	window := application.NewWindow("Incoming E-War preview")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	enableLegacyTestFilters(ui)
	showAllTestGraphs(ui)
	start := time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)
	log := parseEWarTestLog(t)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A", "E-War only"}, First: start.Add(-time.Second), Last: start.Add(5 * time.Second), Logs: []*parsedLog{
		log, {Character: "E-War only", EWar: []ewarEvent{{Second: start.Unix(), Effect: "ECM", Message: "You are jammed by Enemy."}}},
	}}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(6*time.Second))
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	return ui, start
}

func ewarSeries(t *testing.T, ui *analyzerUI, name string) plotSeries {
	t.Helper()
	for _, series := range ui.ewar.data.Series {
		if series.Name == name {
			return series
		}
	}
	t.Fatalf("missing E-War series %q: %+v", name, ui.ewar.data.Series)
	return plotSeries{}
}

func TestEWarParsingPreservesDamageAndArchiveBounds(t *testing.T) {
	log := parseEWarTestLog(t)
	if len(log.EWar) != 5 || log.EWar[0].Amount != 100 || len(log.Points) != 1 || log.Points[0].Incoming != 7 || log.Points[0].IncomingHits != 1 || len(log.Events) != 8 {
		t.Fatalf("E-War changed damage or event records: %+v", log)
	}
	// A log with no damage still supplies the full timeline and survives dedup.
	log.Points = nil
	log.Hits = nil
	snapshot := &logSnapshot{SourceFiles: map[string]cachedLog{"first": {Log: log}, "duplicate": {Log: log}}}
	snapshot.populateLogs()
	if snapshot.Duplicates != 1 || snapshot.First.Unix() != log.EWar[0].Second || snapshot.Last.Unix() != log.EWar[4].Second {
		t.Fatalf("incorrect E-War-only archive bounds: %+v", snapshot)
	}
}

func TestEWarPlotUnitsAveragingFiltersAndNavigation(t *testing.T) {
	ui, start := newEWarTestUI(t)
	if !ui.selected["E-War only"] || len(ui.filterOptions[filterCharacters]) != 2 || len(ui.tableSeries) != 1 || ui.tableSeries[0].TotalIncoming != 7 {
		t.Fatal("E-War-only character missing or damage totals contaminated")
	}
	if !strings.HasPrefix(ui.summary.Text, "2 selected") || !strings.Contains(ui.summary.Text, "Incoming E-War 5 events / 70 GJ") {
		t.Fatalf("E-War footer did not use raw range events: %s", ui.summary.Text)
	}
	if hits := ewarSeries(t, ui, "Energy neutralizer").Hits; len(hits) != 2 || hits[0].Damage != 1 || hits[1].Damage != 1 {
		t.Fatalf("event counts/zero drain lost: %+v", hits)
	}
	ui.ewarMetric.SetSelected(ewarCapacitor)
	if series := ewarSeries(t, ui, "Energy neutralizer"); len(series.Hits) != 2 || series.Hits[0].Damage != 50 || series.Hits[1].Damage != 0 || ui.ewar.data.HitUnit != "GJ" {
		t.Fatalf("incorrect capacitor amounts: %+v", series)
	}
	if len(ui.ewar.data.Series) != 2 {
		t.Fatal("non-capacitor effects leaked into GJ view")
	}
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	if got := ewarSeries(t, ui, "Energy neutralizer").Values[0]; got != 15 {
		t.Fatalf("missing average lookback: %g", got)
	}
	ui.applyGraphRange(start, start.Add(12*time.Second))
	setTestGraphBucket(ui, 5*time.Second)
	values := ewarSeries(t, ui, "Energy neutralizer").Values
	if math.Abs(values[0]-15) > 1e-10 || math.Abs(values[1]-13) > 1e-10 || values[2] != 0 || ui.ewar.data.RateUnit != "GJ/s" {
		t.Fatalf("incorrect coarse/partial bucket average: %v", values)
	}
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	if len(ui.ewar.data.Series) != 2 {
		t.Fatal("incoming damage NPC filters changed E-War")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	if len(ui.ewar.data.Series) != 0 {
		t.Fatal("shared ship filter leaked E-War")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.setDamageFilterOption(filterCharacters, "Pilot A", false)
	if len(ui.ewar.data.Series) != 0 {
		t.Fatal("character filter leaked capacitor events")
	}
	ui.ewarMetric.SetSelected(ewarEvents)
	if len(ui.ewar.data.Series) != 1 || ewarSeries(t, ui, "ECM").Values[0] != .1 {
		t.Fatal("E-War-only character could not be selected independently")
	}
	before := graphHistoryState(ui)
	ui.ewar.OnRangeSelected(start.Add(time.Second), start.Add(3*time.Second))
	ui.undoGraphRange()
	if graphHistoryState(ui) != before || !ui.ewar.data.Start.Equal(before.Start) {
		t.Fatal("E-War range navigation did not synchronize")
	}
	other := fyne.CurrentApp().NewWindow("Restored E-War metric")
	defer other.Close()
	ui.ewarMetric.SetSelected(ewarCapacitor)
	if restored := newAnalyzerUI(other); restored.ewarMetric.Selected != ewarCapacitor {
		t.Fatal("E-War metric did not persist")
	}
}

func TestEWarSharedFiltersUseEventTimeBeforeAveraging(t *testing.T) {
	ui, start := newEWarTestUI(t)
	ui.shipHistory = shipHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot A", TypeID: 1, TypeName: "Loki", GroupID: 963, ClassName: "Strategic Cruiser", FirstSeen: start.Add(-time.Second), LastSeen: start},
		{CharacterID: 1, CharacterName: "Pilot A", TypeID: 2, TypeName: "Thrasher", GroupID: 420, ClassName: "Destroyer", FirstSeen: start.Add(time.Second), LastSeen: start.Add(5 * time.Second)},
	}}
	ui.locationHistory = locationHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30000142, SystemName: "Jita", FirstSeen: start.Add(-time.Second), LastSeen: start},
		{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30002187, SystemName: "Amarr", FirstSeen: start.Add(time.Second), LastSeen: start.Add(5 * time.Second)},
	}}
	ui.characterGroups = []characterGroup{{ID: "fleet", Name: "Fleet", Members: []string{"Pilot A"}}}
	ui.graphAverageWindow = dpsAverageWindow
	ui.ewarMetric.SetSelected(ewarCapacitor)
	ui.rebuildView()
	ui.setAllDamageFilterOptions(filterSystem, false)
	ui.setDamageFilterOption(filterSystem, "system:30002187", true)
	if len(ui.ewar.data.Series) != 1 || ewarSeries(t, ui, "Nosferatu").Values[1] != 2 {
		t.Fatal("system filter leaked prior-location E-War into averages")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:1", true)
	if len(ui.ewar.data.Series) != 0 {
		t.Fatal("ship and system filters did not intersect")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.setAllDamageFilterOptions(filterShipClass, false)
	ui.setDamageFilterOption(filterShipClass, "class:963", true)
	if len(ui.ewar.data.Series) != 0 {
		t.Fatal("class filter leaked E-War from a later ship")
	}
	ui.setAllDamageFilterOptions(filterShipClass, true)
	ui.ewarMetric.SetSelected(ewarEvents)
	ui.setAllDamageFilterOptions(filterCharacterGroup, false)
	ui.setDamageFilterOption(filterCharacterGroup, filterUngrouped, true)
	if len(ui.ewar.data.Series) != 0 {
		t.Fatal("group and system filters did not intersect")
	}
	ui.setAllDamageFilterOptions(filterSystem, true)
	if len(ui.ewar.data.Series) != 1 || ewarSeries(t, ui, "ECM").Values[0] != .1 {
		t.Fatal("ungrouped E-War-only character was lost")
	}
	if len(ui.snapshot.Logs[0].EWar) != 5 {
		t.Fatal("filtering mutated E-War archive")
	}
}

func TestEWarGraphLayoutAndHover(t *testing.T) {
	ui, start := newEWarTestUI(t)
	ui.ewarMetric.SetSelected(ewarCapacitor)
	for _, chart := range []*damageChart{ui.outgoing, ui.incoming, ui.ewar} {
		if chart.Size().Height < 120 {
			t.Fatalf("graph too short at 1440x960: %v", chart.Size())
		}
	}
	ui.graphsScroll.ScrollToBottom()
	at := fyne.CurrentApp().Driver().AbsolutePositionForObject(ui.ewar)
	if at.Y+ui.ewar.Size().Height > ui.window.Canvas().Size().Height {
		t.Fatal("E-War graph cannot be reached by scrolling")
	}
	ui.ewar.MouseMoved(testMouseEvent(chartPlotLeft, 60, 0))
	if got := ui.ewar.hoverReadings[0].Text; !strings.Contains(got, "50 GJ") || !strings.Contains(got, "Pilot A") || !strings.Contains(got, "energy neutralized") || strings.Contains(got, "damage") {
		t.Fatalf("E-War hover has incorrect units/details: %q", got)
	}
	if path := os.Getenv("EVELA_EWAR_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path+".individual.png")
	}
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	ui.ewar.MouseMoved(testMouseEvent(chartPlotLeft, 60, 0))
	if got := ui.ewar.hoverReadings[0].Text; !strings.Contains(got, "15.0 GJ/s (10s avg)") || strings.Contains(got, "DPS") {
		t.Fatalf("average hover units: %q", got)
	}
	ui.applyGraphRange(start, start.Add(1000*time.Second))
	setTestGraphBucket(ui, 5*time.Minute)
	ui.ewarMetric.SetSelected(ewarEvents)
	ui.ewar.MouseMoved(testMouseEvent(chartPlotLeft, 60, 0))
	if got := ui.ewar.hoverReadings[0].Text; strings.Contains(got, ": 0.0 events/s") || !strings.Contains(got, "events/s") {
		t.Fatalf("small event rate rounded away: %q", got)
	}
	ui.ewarMetric.SetSelected(ewarCapacitor)
	ui.ewar.OnWheelZoom(start, start.Add(3*time.Second))
	if !ui.incoming.data.End.Equal(ui.ewar.data.End) || !ui.outgoing.data.End.Equal(ui.ewar.data.End) {
		t.Fatal("wheel zoom did not synchronize all graphs")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_EWAR_CAPTURE"))
}
