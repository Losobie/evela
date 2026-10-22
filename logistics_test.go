package main

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/encoding/unicode"
)

const logisticsTestLog = "Listener: Logi\n" +
	"[ 2026.10.20 02:00:01 ] (combat) 20 remote shield boosted to Fleetmate - Remote Shield Booster\n" +
	"[ 2026.10.20 01:59:59 ] (combat) 100 remote armor repaired to Fleetmate - Remote Armor Repairer\n" +
	"[ 2026.10.20 02:00:00 ] (combat) <color=green><b>50</b></color> remote armor repaired to Fleetmate - Remote Armor Repairer\n" +
	"[ 2026.10.20 02:00:00 ] (combat) 0 remote armor repaired to Fleetmate - Remote Armor Repairer\n" +
	"[ 2026.10.20 02:00:02 ] (combat) 311 remote capacitor transmitted to Guardian Pilot - Large Inductive Compact Remote Capacitor Transmitter\n" +
	"[ 2026.10.20 02:00:03 ] (combat) 291 remote capacitor transmitted by Guardian Pilot - Large Inductive Compact Remote Capacitor Transmitter\n" +
	"[ 2026.10.20 02:00:04 ] (combat) 323 remote armor repaired by Guardian Pilot - Large Remote Armor Repairer II\n" +
	"[ 2026.10.20 02:00:05 ] (combat) 127 remote shield boosted by Fleetmate - Remote Shield Booster\n" +
	"[ 2026.10.20 02:00:06 ] (combat) +10 GJ energy drained from Enemy - Nosferatu\n" +
	"[ 2026.10.20 02:00:06 ] (combat) 50 GJ energy neutralized Enemy - Neutralizer\n" +
	"[ 2026.10.20 02:00:06 ] (notify) 999 remote armor repaired to Fleetmate - Not combat\n" +
	"[ 2026.10.20 02:00:07 ] (combat) 7 from Enemy - Missile - Hits\n"

func TestParseLogisticsDirectionAmountsAndEncoding(t *testing.T) {
	for _, tc := range []struct {
		message, kind string
		amount        float64
		incoming      bool
	}{
		{"311 remote capacitor transmitted to Guardian Pilot - Transmitter", "Capacitor transfer", 311, false},
		{"291 remote capacitor transmitted by Guardian Pilot - Transmitter", "Capacitor transfer", 291, true},
		{"323 remote armor repaired to Fleetmate - Repairer", "Armor repair", 323, false},
		{"323 remote armor repaired by Fleetmate - Repairer", "Armor repair", 323, true},
		{"127 remote shield boosted to Fleetmate - Booster", "Shield repair", 127, false},
		{"127 remote shield boosted by Fleetmate - Booster", "Shield repair", 127, true},
		{"1,234.5 remote armor repaired from Fleetmate - Repairer", "Armor repair", 1234.5, true},
		{"0 remote shield boosted by Fleetmate - Booster", "Shield repair", 0, true},
		{"20 GJ remote capacitor transmitted to Fleetmate", "Capacitor transfer", 20, false},
		{"0 remote armor repaired to Fleetmate", "Armor repair", 0, false},
		{"100 remote hull repaired to Fleetmate", "", 0, false},
		{"100 armor repaired by Armor Repairer II", "", 0, false},
		{"100 shield boosted by Shield Booster II", "", 0, false},
		{"+10 GJ energy drained from Enemy - Nosferatu", "", 0, false},
		{"-10 GJ energy drained to Enemy - Nosferatu", "", 0, false},
		{"10 GJ energy neutralized Enemy - Neutralizer", "", 0, false},
		{"100 from Remote Armor Repairer - Missile - Hits", "", 0, false},
		{"-20 remote shield boosted to Fleetmate", "", 0, false},
		{"1.2.3 remote armor repaired to Fleetmate", "", 0, false},
		{"200 remote armor repaired to ", "", 0, false},
		{"Your Remote Armor Repairer cannot be activated.", "", 0, false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			event, ok := parseLogistics(tc.message, 100)
			if ok != (tc.kind != "") || event.Kind != tc.kind || event.Amount != tc.amount || event.Incoming != tc.incoming {
				t.Fatalf("event=%+v accepted=%v", event, ok)
			}
		})
	}
	for _, order := range []unicode.Endianness{unicode.LittleEndian, unicode.BigEndian} {
		data, err := unicode.UTF16(order, unicode.UseBOM).NewEncoder().Bytes([]byte(logisticsTestLog))
		if err != nil {
			t.Fatal(err)
		}
		log, err := parseLog(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(log.Logistics) != 8 || log.Logistics[0].Amount != 100 || len(log.EWar) != 1 || len(log.Points) != 1 || log.Points[0].Incoming != 7 || len(log.Events) != 12 {
			t.Fatalf("logistics changed damage/E-War/events or lost encoded input: %+v", log)
		}
	}
}

func newLogisticsTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	application := test.NewTempApp(t)
	window := application.NewWindow("Logistics graphs preview")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	showAllTestGraphs(ui)
	// Keep the original repair-specific assertions focused on HP; the new
	// mixed-Type and default-All behavior is covered separately.
	ui.logisticsTypes[graphOutgoingLogistics].set("Capacitor transfer", false)
	ui.logisticsTypes[graphIncomingLogistics].set("Capacitor transfer", false)
	log, err := parseLog(strings.NewReader(logisticsTestLog))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Logi", "Logistics only"}, Logs: []*parsedLog{log,
		{Character: "Logistics only", Logistics: []logisticsEvent{{Second: start.Unix(), Kind: "Shield repair", Amount: 10}}},
	}, First: start.Add(-time.Second), Last: start.Add(7 * time.Second)}
	ui.graphAverage10s = false
	ui.applyGraphRange(start, start.Add(8*time.Second))
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	return ui, start
}

func selectLogisticsTestTypes(ui *analyzerUI, id string, kinds ...string) {
	ui.logisticsTypes[id].Default, ui.logisticsTypes[id].Values = false, map[string]bool{}
	for _, kind := range kinds {
		ui.logisticsTypes[id].set(kind, true)
	}
	ui.saveLogisticsTypes(id)
}

func findLogisticsSeries(t *testing.T, chart *damageChart, name string) plotSeries {
	t.Helper()
	for _, series := range chart.data.Series {
		if series.Name == name {
			return series
		}
	}
	t.Fatalf("missing logistics series %q: %+v", name, chart.data.Series)
	return plotSeries{}
}

func TestLogisticsPlotsKeepDirectionsUnitsAndAveragesSeparate(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	if !ui.selected["Logistics only"] || len(ui.filterOptions[filterCharacters]) != 2 || !strings.HasPrefix(ui.summary.Text, "2 selected") || len(ui.tableSeries) != 1 || ui.tableSeries[0].TotalIncoming != 7 {
		t.Fatal("logistics-only character lost or damage totals contaminated")
	}
	if hits := findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Hits; len(hits) != 2 || hits[0].Damage != 50 || hits[1].Damage != 0 {
		t.Fatalf("raw armor reps: %+v", hits)
	}
	if hits := findLogisticsSeries(t, ui.incomingLogistics, "Shield repair").Hits; len(hits) != 1 || hits[0].Damage != 127 {
		t.Fatalf("raw incoming shield: %+v", hits)
	}
	selectLogisticsTestTypes(ui, graphOutgoingLogistics, "Capacitor transfer")
	if ui.logisticsTypes[graphIncomingLogistics].selected("Capacitor transfer") || ui.outgoingLogistics.data.HitUnit != "GJ" || findLogisticsSeries(t, ui.outgoingLogistics, "Capacitor transfer").Hits[0].Damage != 311 {
		t.Fatal("metric changed wrong graph or mixed HP and GJ")
	}
	selectLogisticsTestTypes(ui, graphIncomingLogistics, "Capacitor transfer")
	if findLogisticsSeries(t, ui.incomingLogistics, "Capacitor transfer").Hits[0].Damage != 291 {
		t.Fatal("incoming capacitor direction wrong")
	}
	ui.graphAverage10s = true
	selectLogisticsTestTypes(ui, graphOutgoingLogistics, "Shield repair", "Armor repair")
	if findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Values[0] != 15 || ui.outgoingLogistics.data.RateUnit != "HP/s" {
		t.Fatal("armor average lost lookback or units")
	}
	if math.Abs(findLogisticsSeries(t, ui.incomingLogistics, "Capacitor transfer").Values[3]-29.1) > 1e-10 || ui.incomingLogistics.data.RateUnit != "GJ/s" {
		t.Fatal("incoming capacitor average wrong")
	}
	ui.applyGraphRange(start, start.Add(12*time.Second))
	ui.bucketSelect.SetSelected("5 seconds")
	if values := findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Values; math.Abs(values[0]-15) > 1e-10 || math.Abs(values[1]-13) > 1e-10 || values[2] != 0 {
		t.Fatalf("coarse/partial armor buckets: %v", values)
	}
	ui.setDamageFilterOption(filterCharacters, "Logi", false)
	if len(ui.outgoingLogistics.data.Series) != 1 || findLogisticsSeries(t, ui.outgoingLogistics, "Shield repair").Values[0] != 1 || len(ui.incomingLogistics.data.Series) != 0 {
		t.Fatal("character filter leaked logistics")
	}
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	if len(ui.outgoingLogistics.data.Series) != 1 {
		t.Fatal("damage NPC filters changed logistics")
	}
	ui.setDamageFilterOption(filterCharacters, "Logi", true)
	selectLogisticsTestTypes(ui, graphOutgoingLogistics, "Capacitor transfer")
	selectLogisticsTestTypes(ui, graphIncomingLogistics, "Shield repair", "Armor repair")
	other := fyne.CurrentApp().NewWindow("Restored logistics")
	defer other.Close()
	restored := newAnalyzerUI(other)
	if !restored.logisticsTypes[graphOutgoingLogistics].selected("Capacitor transfer") || restored.logisticsTypes[graphOutgoingLogistics].selected("Armor repair") || restored.logisticsTypes[graphIncomingLogistics].selected("Capacitor transfer") {
		t.Fatal("independent logistics metrics did not persist")
	}
}

func TestLogisticsOnlyArchiveBoundsAndDuplicateExports(t *testing.T) {
	log, err := parseLog(strings.NewReader("Listener: Logistics only\n[ 2026.10.20 01:59:59 ] (combat) 0 remote armor repaired to Fleetmate\n[ 2026.10.20 02:00:03 ] (combat) 100 remote capacitor transmitted by Fleetmate\n"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &logSnapshot{SourceFiles: map[string]cachedLog{"first": {Log: log}, "copy": {Log: log}}}
	snapshot.populateLogs()
	if len(snapshot.Logs) != 1 || snapshot.Duplicates != 1 || snapshot.First.Unix() != log.Logistics[0].Second || snapshot.Last.Unix() != log.Logistics[1].Second {
		t.Fatalf("wrong logistics-only bounds/dedup: %+v", snapshot)
	}
	start, end := presetRange("All history", snapshot)
	if start != snapshot.First || end != snapshot.Last.Add(time.Second) {
		t.Fatal("all history excluded logistics-only data")
	}
}

func TestLogisticsSharedFiltersApplyAtEventTime(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	ui.snapshot.Logs[0].Logistics = []logisticsEvent{{Second: start.Unix(), Kind: "Armor repair", Amount: 100}, {Second: start.Add(time.Minute).Unix(), Kind: "Armor repair", Amount: 200}}
	ui.snapshot.Logs[1].Logistics = []logisticsEvent{{Second: start.Add(20 * time.Second).Unix(), Kind: "Armor repair", Amount: 300}}
	ui.locationHistory = locationHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30000142, SystemName: "Jita", FirstSeen: start, LastSeen: start.Add(5 * time.Second)},
		{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30002187, SystemName: "Amarr", FirstSeen: start.Add(time.Minute), LastSeen: start.Add(65 * time.Second)},
	}, 2: {{CharacterID: 2, CharacterName: "Pilot B", SystemID: 30000142, SystemName: "Jita", FirstSeen: start, LastSeen: start.Add(30 * time.Second)}}}
	ui.graphAverage10s = true
	ui.rebuildView()
	ui.setAllDamageFilterOptions(filterSystem, false)
	ui.setDamageFilterOption(filterSystem, "system:30000142", true)
	if values := findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Values; values[0] != 10 || values[20] != 30 || values[60] != 0 {
		t.Fatalf("system filter leaked logistics: %v", values)
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:2", true)
	if len(ui.outgoingLogistics.data.Series) != 0 {
		t.Fatal("ship/system filters did not intersect")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.setAllDamageFilterOptions(filterShipClass, false)
	ui.setDamageFilterOption(filterShipClass, "class:420", true)
	if len(ui.outgoingLogistics.data.Series) != 0 {
		t.Fatal("ship class filter leaked logistics")
	}
	ui.setAllDamageFilterOptions(filterShipClass, true)
	ui.setAllDamageFilterOptions(filterCharacterGroup, false)
	ui.setDamageFilterOption(filterCharacterGroup, "group:scouts", true)
	if values := findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Values; values[0] != 10 || values[20] != 0 {
		t.Fatal("character group filter leaked logistics")
	}
	if len(ui.snapshot.Logs[0].Logistics) != 2 {
		t.Fatal("filtering mutated logistics archive")
	}
}

func TestLogisticsGraphsScrollLayoutHoverAndNavigation(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	cards := ui.graphsScroll.Content.(*fyne.Container).Objects
	want := []string{"Outgoing damage", "Outgoing logistics", "Incoming damage", "Incoming logistics", "Incoming E-War"}
	if len(cards) != len(want) {
		t.Fatal("incorrect graph count")
	}
	for i, object := range cards {
		if object.(*widget.Card).Title != want[i] {
			t.Fatal("incorrect graph order")
		}
	}
	if ui.graphsScroll.Direction != container.ScrollVerticalOnly || ui.graphsScroll.Content.Size().Height <= ui.graphsScroll.Size().Height {
		t.Fatal("graphs do not have vertical overflow/scrolling")
	}
	for _, chart := range []*damageChart{ui.outgoing, ui.outgoingLogistics, ui.incoming, ui.incomingLogistics, ui.ewar} {
		if chart.Size().Height < 175 {
			t.Fatal("scrolling compressed plot height")
		}
	}
	ui.outgoingLogistics.MouseMoved(testMouseEvent(chartPlotLeft, 60, 0))
	if text := ui.outgoingLogistics.hoverReadings[0].Text; !strings.Contains(text, "50 HP") || !strings.Contains(text, "Logi") || !strings.Contains(text, "remote armor repaired") || strings.Contains(text, "damage") {
		t.Fatalf("raw logistics hover: %q", text)
	}
	ui.graphAverage10s = true
	ui.renderSelection()
	ui.outgoingLogistics.MouseMoved(testMouseEvent(chartPlotLeft, 60, 0))
	if text := ui.outgoingLogistics.hoverReadings[0].Text; !strings.Contains(text, "15.0 HP/s (10s avg)") || strings.Contains(text, "DPS") {
		t.Fatalf("averaged logistics hover: %q", text)
	}
	if path := os.Getenv("EVELA_LOGISTICS_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path+".top.png")
	}
	ui.graphsScroll.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -300)})
	if ui.graphsScroll.Offset.Y <= 0 {
		t.Fatal("wheel on panel did not scroll")
	}
	ui.graphsScroll.ScrollToBottom()
	position := fyne.CurrentApp().Driver().AbsolutePositionForObject(ui.ewar)
	bounds := fyne.CurrentApp().Driver().AbsolutePositionForObject(ui.graphsScroll)
	if position.Y < bounds.Y || position.Y+ui.ewar.Size().Height > bounds.Y+ui.graphsScroll.Size().Height+1 {
		t.Fatal("last graph cannot be reached")
	}
	before := ui.graphsScroll.Offset
	selectLogisticsTestTypes(ui, graphIncomingLogistics, "Capacitor transfer")
	if ui.graphsScroll.Offset != before {
		t.Fatal("metric change reset scroll position")
	}
	ui.incomingLogistics.OnRangeSelected(start.Add(time.Second), start.Add(6*time.Second))
	for _, chart := range []*damageChart{ui.outgoing, ui.incoming, ui.outgoingLogistics, ui.incomingLogistics, ui.ewar} {
		if !chart.data.Start.Equal(start.Add(time.Second)) {
			t.Fatal("logistics drag did not synchronize graphs")
		}
	}
	ui.undoGraphRange()
	if !ui.outgoingLogistics.data.Start.Equal(start) {
		t.Fatal("logistics zoom undo failed")
	}
	ui.outgoingLogistics.OnWheelZoom(start, start.Add(4*time.Second))
	if !ui.incomingLogistics.data.End.Equal(start.Add(4 * time.Second)) {
		t.Fatal("logistics wheel zoom did not synchronize")
	}
	ui.undoGraphRange()
	if path := os.Getenv("EVELA_LOGISTICS_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path+".bottom.png")
		ui.graphsScroll.ScrollToOffset(fyne.NewPos(0, cards[2].Position().Y))
		captureWindowPreview(t, ui.window, path+".incoming.png")
	}
}
