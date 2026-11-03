package main

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestLegacyHistoryClassMetadataResolvesWithoutConnections(t *testing.T) {
	f := newESIFixture(t)
	start := time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)
	f.service.config.ShipHistory = shipHistory{123: {
		{CharacterID: 123, CharacterName: "Disconnected Pilot", TypeID: 29990, TypeName: "Loki", FirstSeen: start, LastSeen: start.Add(time.Minute), Observations: 3},
		{CharacterID: 123, CharacterName: "Disconnected Pilot", TypeID: 29984, FirstSeen: start.Add(2 * time.Minute), LastSeen: start.Add(3 * time.Minute), Observations: 3},
	}}
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.shipRequests.Load() != 0 || f.typeRequests.Load() != 2 || f.groupRequests.Load() != 1 {
		t.Fatal("historical metadata used private ship access or missed the class cache")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	periods := loaded.config.ShipHistory[123]
	if periods[0].ClassName != "Strategic Cruiser" || periods[1].ClassName != "Strategic Cruiser" || periods[1].TypeName != "Tengu" || !periods[0].FirstSeen.Equal(start) || !periods[0].LastSeen.Equal(start.Add(time.Minute)) || periods[0].Observations != 3 {
		t.Fatalf("legacy metadata migration lost or rewrote observed history: %+v", periods)
	}
}

// UI navigation fixtures use one known ship throughout their test interval.
// Production history always comes from the ESI worker's observed periods.
func applyTestShipRecords(ui *analyzerUI, records []shipRecord) {
	ui.shipHistory = shipHistory{}
	for index, record := range records {
		if !record.current() {
			continue
		}
		id := record.CharacterID
		if id <= 0 {
			id = int64(index + 1)
		}
		ui.shipHistory[id] = []shipPeriod{{CharacterID: id, CharacterName: record.CharacterName,
			TypeID: record.TypeID, TypeName: record.TypeName, ClassName: record.ClassName, GroupID: record.GroupID,
			FirstSeen: ui.view.Start.Add(-24 * time.Hour), LastSeen: ui.view.End.Add(24 * time.Hour)}}
	}
	ui.applyShipRecords(records)
}

func TestHistoricalShipEventsSplitWithinBucketsAndAverages(t *testing.T) {
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{Characters: []string{"A", "B", "Outside"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{
			{Second: start.Unix() - 1, Outgoing: 100},
			{Second: start.Unix(), Outgoing: 10, Incoming: 1, OutgoingHits: 1},
			{Second: start.Unix() + 2, Outgoing: 20, Incoming: 2, OutgoingHits: 1},
			{Second: start.Unix() + 3, Outgoing: 30, Incoming: 3, OutgoingHits: 1}, // Assume the previous hull until the new sighting.
			{Second: start.Unix() + 4, Outgoing: 40, Incoming: 4, OutgoingHits: 1},
			{Second: start.Unix() + 5, Outgoing: 50, Incoming: 5, OutgoingHits: 1},
			{Second: start.Unix() + 8, Outgoing: 80, Incoming: 8, OutgoingHits: 1}, // Within 45 seconds of the last sighting.
			{Second: start.Unix() + 9, Outgoing: 90},                               // Exclusive range end.
		}},
		{Character: "B", Points: []damagePoint{{Second: start.Unix(), Outgoing: 20, Incoming: 2, OutgoingHits: 1}}},
		{Character: "Outside", Points: []damagePoint{{Second: start.Unix() - 1, Outgoing: 9999}}},
	}}
	history := shipHistory{
		1: {
			{CharacterID: 1, CharacterName: "A", TypeID: 1, TypeName: "Thrasher", ClassName: "Destroyer", FirstSeen: start.Add(-time.Second), LastSeen: start.Add(2 * time.Second)},
			{CharacterID: 1, CharacterName: "A", TypeID: 2, TypeName: "Caracal", ClassName: "Cruiser", FirstSeen: start.Add(4 * time.Second), LastSeen: start.Add(7 * time.Second)},
		},
		2: {{CharacterID: 2, CharacterName: "B", TypeID: 3, TypeName: "Cormorant", ClassName: "Destroyer", FirstSeen: start, LastSeen: start.Add(2 * time.Second)}},
	}
	view := buildView(snapshot, start, start.Add(9*time.Second), 5*time.Second)
	byName := func(series []characterSeries) map[string]characterSeries {
		result := map[string]characterSeries{}
		for _, item := range series {
			result[item.Name] = item
		}
		return result
	}
	groups := byName(view.historicalShipSeries(snapshot, history, nil, false, 0))
	if len(groups) != 3 || groups["Thrasher"].TotalOutgoing != 60 || groups["Caracal"].TotalOutgoing != 170 || groups["Cormorant"].TotalOutgoing != 20 {
		t.Fatalf("events were not assigned to their historical hulls: %+v", groups)
	}
	if !reflect.DeepEqual(groups["Caracal"].Outgoing, []float64{8, 32.5}) || !reflect.DeepEqual(groups["Thrasher"].Outgoing, []float64{12, 0}) || groups["Caracal"].OutgoingHits != 3 {
		t.Fatal("intra-bucket ship changes, partial bucket rates or unknown hits are incorrect")
	}
	classes := byName(view.historicalShipSeries(snapshot, history, nil, true, 0))
	if len(classes) != 2 || classes["Destroyer"].TotalOutgoing != 80 || !reflect.DeepEqual(classes["Destroyer"].MemberCounts, []int{2, 0}) || !reflect.DeepEqual(classes["Cruiser"].MemberCounts, []int{1, 1}) {
		t.Fatal("class grouping did not merge hulls or preserve bucket membership")
	}
	selected := byName(view.historicalShipSeries(snapshot, history, map[string]bool{"A": true}, true, 0))
	if selected["Destroyer"].TotalOutgoing != 60 || len(selected["Destroyer"].Members) != 1 || selected["Destroyer"].MemberCounts[0] != 1 {
		t.Fatal("historical class groups ignored character selection")
	}
	// Compare coarse historical rolling averages with the mean of one-second
	// historical rates. A's old hull must retain its pre-range damage tail.
	averages := byName(view.historicalShipSeries(snapshot, history, nil, false, dpsAverageWindow))
	if math.Abs(averages["Thrasher"].Outgoing[0]-13.4) > 1e-10 || averages["Thrasher"].Outgoing[1] != 16 || averages["Caracal"].Outgoing[0] != .8 || averages["Caracal"].Outgoing[1] != 11 {
		t.Fatalf("rolling damage moved to the new hull or used the wrong denominator: %+v", averages)
	}
	if averages["Cormorant"].Outgoing[0] != 2 || averages["Cormorant"].Outgoing[1] != 2 {
		t.Fatal("averaging lost a second character or included the out-of-range character")
	}
	// Duplicate names with different IDs stay unknown, even with matching hulls.
	history[3] = append([]shipPeriod(nil), history[1]...)
	ambiguous := byName(view.historicalShipSeries(snapshot, history, map[string]bool{"A": true}, false, 0))
	if len(ambiguous) != 1 || ambiguous["Unknown ship"].TotalOutgoing != 230 {
		t.Fatalf("ambiguous names were assigned a hull: %+v", ambiguous)
	}
}

func TestHistoricalShipAssumptionExpiresForTypeAndClass(t *testing.T) {
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{Characters: []string{"Pilot"}, Logs: []*parsedLog{{Character: "Pilot", Points: []damagePoint{
		{Second: start.Unix() - 1, Outgoing: 40}, // Before any observation.
		{Second: start.Unix(), Outgoing: 10},
		{Second: start.Unix() + 45, Outgoing: 20},
		{Second: start.Unix() + 46, Outgoing: 30}, // Beyond the assumption window.
	}}}}
	history := shipHistory{123: {{CharacterID: 123, CharacterName: "Pilot", TypeID: 29990,
		TypeName: "Loki", ClassName: "Strategic Cruiser", FirstSeen: start, LastSeen: start}}}
	view := buildView(snapshot, start.Add(-time.Second), start.Add(56*time.Second), time.Second)
	for _, byClass := range []bool{false, true} {
		for _, average := range []time.Duration{0, dpsAverageWindow} {
			known, unknown := "Loki", "Unknown ship"
			if byClass {
				known, unknown = "Strategic Cruiser", "Unknown ship class"
			}
			groups := map[string]characterSeries{}
			for _, series := range view.historicalShipSeries(snapshot, history, nil, byClass, average) {
				groups[series.Name] = series
			}
			if len(groups) != 2 || groups[known].TotalOutgoing != 30 || groups[unknown].TotalOutgoing != 70 {
				t.Fatalf("type/class grouping or averaging ignored the 45-second cutoff: class=%v, average=%v, groups=%+v", byClass, average, groups)
			}
			if average > 0 && groups[known].Outgoing[47] != 2 {
				t.Fatal("rolling damage lost its assumed ship when its tail crossed the cutoff")
			}
		}
	}
}

func TestGraphGroupingUsesHistoryWithoutCurrentConnections(t *testing.T) {
	application := test.NewTempApp(t)
	application.Preferences().SetBool(graphAveragePreference, false)
	window := application.NewWindow("Historical damage groups")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"A", "B"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 10, Incoming: 1}, {Second: start.Unix() + 5, Outgoing: 20, Incoming: 2}}},
		{Character: "B", Points: []damagePoint{{Second: start.Unix(), Outgoing: 30, Incoming: 3}}},
	}}
	ui.shipHistory = shipHistory{
		1: {
			{CharacterID: 1, CharacterName: "A", TypeID: 1, TypeName: "Thrasher", ClassName: "Destroyer", FirstSeen: start, LastSeen: start.Add(2 * time.Second)},
			{CharacterID: 1, CharacterName: "A", TypeID: 2, TypeName: "Caracal", ClassName: "Cruiser", FirstSeen: start.Add(5 * time.Second), LastSeen: start.Add(9 * time.Second)},
		},
		2: {{CharacterID: 2, CharacterName: "B", TypeID: 3, TypeName: "Cormorant", ClassName: "Destroyer", FirstSeen: start, LastSeen: start.Add(2 * time.Second)}},
	}
	ui.selected["A"], ui.selected["B"] = true, true
	ui.applyGraphRange(start, start.Add(10*time.Second))
	ui.groupSelect.SetSelected("Ship class history")
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	if len(ui.outgoing.data.Series) != 2 || ui.outgoing.data.Series[1].Name != "Destroyer" || ui.outgoing.data.Series[1].Values[0] != 40 || ui.incoming.data.Series[0].Values[5] != 2 {
		t.Fatal("graphs did not use the ship at each event time")
	}
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+1, 80, 0))
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, "Destroyer · B: 30 damage") {
		t.Fatal("individual-hit hover lost historical class attribution")
	}
	// A current ship snapshot must not rewrite old damage, and disconnected
	// characters keep their historical groups after refreshing.
	ui.applyShipRecords([]shipRecord{{CharacterName: "A", TypeID: 99, TypeName: "Avatar", ClassName: "Titan", CheckedAt: time.Now()}})
	ui.applyShipRecords(nil)
	ui.updateRange()
	if len(ui.tableSeries) != 2 || ui.tableSeries[0].Name != "Cruiser" || ui.tableSeries[0].TotalOutgoing != 20 || ui.tableSeries[1].TotalOutgoing != 40 || len(characterFilterChecks(ui.characters)) != 2 {
		t.Fatal("current connections changed historical totals or legend membership")
	}
	ui.characterSearch.SetText("thrasher")
	if got := characterFilterChecks(ui.characters); !reflect.DeepEqual(got, map[string]bool{"A": true}) {
		t.Fatalf("class legend could not find the historical hull: %v", got)
	}
	ui.characterSearch.SetText("avatar")
	if len(characterFilterChecks(ui.characters)) != 0 {
		t.Fatal("historical class legend searched a current hull instead")
	}
	ui.characterSearch.SetText("")
	ui.groupSelect.SetSelected("Ship history")
	if len(ui.outgoing.data.Series) != 3 || ui.outgoing.data.Series[0].Name != "Caracal" || ui.outgoing.data.Series[1].Name != "Cormorant" || ui.outgoing.data.Series[2].Name != "Thrasher" {
		t.Fatal("ship-type grouping did not follow hull changes")
	}
}
