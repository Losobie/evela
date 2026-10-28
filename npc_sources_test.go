package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"golang.org/x/text/encoding/unicode"
)

func TestNPCSourceResolution(t *testing.T) {
	for _, tc := range []struct {
		text            string
		name            string
		npc             bool
		typeID, groupID int
	}{
		{"Guristas Blackbird - Guristas Inferno Heavy Missile - Hits", "Guristas Blackbird", true, 76045, 1568},
		{"Amarr Sentry Gun - Glances Off", "Amarr Sentry Gun", true, 1194, 99},
		{"Sansha's Lord - Mjolnir Cruise Missile - Hits", "Sansha's Lord", true, 11913, 565},
		{"  amarr sentry gun - Hits", "amarr sentry gun", true, 1194, 99},
		{"アマーセントリーガン - Hits", "アマーセントリーガン", true, 1194, 99},
		{"Guristas Blackbird[CORP](Raven) - Missile (Faction) - Hits", "Guristas Blackbird[CORP](Raven)", false, 0, 0},
		{"Moon - Mining[CORP](Tatara) - Standup Heavy Missile - Hits", "Moon - Mining[CORP](Tatara)", false, 0, 0},
		{"Brand new unknown rat - Laser - Hits", "Brand new unknown rat", false, 0, 0},
		{"Amarr Sentry Gun - Unknown variant - Laser - Hits", "Amarr Sentry Gun - Unknown variant", false, 0, 0},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := incomingDamageSource(tc.text)
			if got.Name != tc.name || got.NPC != tc.npc || got.TypeID != tc.typeID || got.GroupID != tc.groupID {
				t.Fatalf("source resolution: %+v", got)
			}
		})
	}
	if catalog := loadNPCCatalog(); catalog.Build == 0 || len(catalog.names) < 7000 || catalog.Groups[99] != "Sentry Gun" {
		t.Fatal("embedded official NPC lookup is incomplete")
	}
}

func TestNPCDuplicateNamesRetainOnlyUnambiguousIDs(t *testing.T) {
	catalog := &npcCatalog{}
	err := json.Unmarshal([]byte(`{"types":[{"id":1,"group":10,"names":["Shared group"]},{"id":2,"group":10,"names":["Shared group"]},{"id":3,"group":11,"names":["Ambiguous"]},{"id":4,"group":12,"names":["Ambiguous"]},{"id":5,"group":12,"names":["Ambiguous"]}]}`), catalog)
	if err != nil {
		t.Fatal(err)
	}
	catalog.index()
	if got := catalog.names["shared group"]; got.TypeID != 0 || got.GroupID != 10 {
		t.Fatalf("shared group: %+v", got)
	}
	if got := catalog.names["ambiguous"]; got.TypeID != 0 || got.GroupID != 0 {
		t.Fatalf("ambiguous type: %+v", got)
	}
}

const npcFilterLog = "Listener: Pilot A\nSession Started: 2026.10.19 02:00:00\n" +
	"[ 2026.10.19 01:59:59 ] (combat) 100 from Amarr Sentry Gun - Hits\n" +
	"[ 2026.10.19 02:00:00 ] (combat) <b>20</b> from Guristas Blackbird - Missile - Hits\n" +
	"[ 2026.10.19 02:00:00 ] (combat) 30 from Guristas Blackbird - Missile - Smashes\n" +
	"[ 2026.10.19 02:00:00 ] (combat) 40 from Amarr Sentry Gun - Hits\n" +
	"[ 2026.10.19 02:00:00 ] (combat) 60 from Guristas Blackbird[CORP](Raven) - Missile - Hits\n" +
	"[ 2026.10.19 02:00:00 ] (combat) 25 from Unknown attacker - Laser - Hits\n" +
	"[ 2026.10.19 02:00:00 ] (combat) 70 to Enemy - Laser - Hits\n" +
	"[ 2026.10.19 02:00:01 ] (combat) 0 from Amarr Sentry Gun - Hits\n"

func TestParseNPCSourcesPreservesSameSecondContributions(t *testing.T) {
	for _, utf16 := range []bool{false, true} {
		input := []byte(npcFilterLog)
		if utf16 {
			input, _ = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(input)
		}
		log, err := parseLog(strings.NewReader(string(input)))
		if err != nil {
			t.Fatal(err)
		}
		if len(log.Sources) != 4 || len(log.IncomingSources) != 6 {
			t.Fatalf("sources not interned/aggregated: %+v", log)
		}
		if got := log.Points[1]; got.Incoming != 175 || got.IncomingHits != 5 || got.Outgoing != 70 {
			t.Fatalf("damage totals changed: %+v", got)
		}
		var damage float64
		var hits int
		for _, contribution := range log.IncomingSources {
			damage += contribution.Damage
			hits += contribution.Hits
		}
		if damage != 275 || hits != 7 {
			t.Fatalf("source data lost damage/hits: %f, %d", damage, hits)
		}
	}
}

func newNPCFilterUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	ui, start := newDamageFilterTestUI(t)
	log, err := parseLog(strings.NewReader(npcFilterLog))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start.Add(-time.Second), Last: start.Add(time.Second)}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(2*time.Second))
	return ui, start
}

func npcIncomingTotal(ui *analyzerUI) float64 {
	var total float64
	for _, series := range ui.tableSeries {
		total += series.TotalIncoming
	}
	return total
}

func TestNPCFiltersIntersectWithoutChangingOutgoingDamage(t *testing.T) {
	ui, start := newNPCFilterUI(t)
	if len(ui.filterOptions[filterNPCNames]) != 3 || len(ui.filterOptions[filterNPCTypes]) != 3 || npcIncomingTotal(ui) != 175 {
		t.Fatal("missing NPC/other options or unfiltered damage")
	}
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	ui.setDamageFilterOption(filterNPCNames, "npc-name:amarr sentry gun", true)
	if npcIncomingTotal(ui) != 40 || filteredTotal(ui) != 70 || ui.tableSeries[0].IncomingHits != 2 || ui.tableSeries[0].OutgoingHits != 1 {
		t.Fatalf("name filter lost same-second damage or zero hits: %+v", ui.tableSeries)
	}
	ui.setAllDamageFilterOptions(filterNPCTypes, false)
	ui.setDamageFilterOption(filterNPCTypes, "npc-group:1568", true)
	if npcIncomingTotal(ui) != 0 || filteredTotal(ui) != 70 {
		t.Fatal("name/type categories did not intersect")
	}
	ui.setAllDamageFilterOptions(filterNPCNames, true)
	if npcIncomingTotal(ui) != 50 {
		t.Fatal("group filter failed")
	}
	ui.setAllDamageFilterOptions(filterNPCTypes, false)
	ui.setDamageFilterOption(filterNPCTypes, filterOtherSources, true)
	if npcIncomingTotal(ui) != 85 {
		t.Fatal("player and unresolved damage was lost or misidentified")
	}
	ui.setAllDamageFilterOptions(filterNPCTypes, true)
	ui.setAllDamageFilterOptions(filterNPCNames, false)
	ui.setDamageFilterOption(filterNPCNames, "npc-name:amarr sentry gun", true)
	ui.graphAverageWindow = dpsAverageWindow
	ui.rebuildView()
	if got := ui.incoming.data.Series[0].Values[0]; got != 14 {
		t.Fatalf("average lost pre-range selected NPC damage: %f", got)
	}
	if got := ui.outgoing.data.Series[0].Values[0]; got != 7 {
		t.Fatalf("outgoing average changed: %f", got)
	}
	ui.groupSelect.SetSelected("Ship class history")
	var groupedRate float64
	for _, series := range ui.incoming.data.Series {
		groupedRate += series.Values[0]
	}
	if npcIncomingTotal(ui) != 40 || filteredTotal(ui) != 70 || groupedRate != 14 {
		t.Fatal("ship grouping ignored NPC filters")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	if npcIncomingTotal(ui) != 0 || filteredTotal(ui) != 0 {
		t.Fatal("NPC and shared filters did not intersect")
	}
	if original := ui.snapshot.Logs[0].Points[1]; original.Incoming != 175 || original.IncomingHits != 5 {
		t.Fatal("filtering mutated parsed data")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.applyGraphRange(start, start.Add(time.Second))
	ui.undoGraphRange()
	if !ui.damageFilterSelected(filterNPCNames, "npc-name:amarr sentry gun") || ui.damageFilterSelected(filterNPCNames, "npc-name:guristas blackbird") {
		t.Fatal("range undo lost NPC selection")
	}
}

func TestNPCFilterPopupSearchPersistenceAndPreview(t *testing.T) {
	ui, start := newNPCFilterUI(t)
	test.Tap(ui.filterButtons[filterNPCNames])
	popup := ui.filterPopup
	popup.search.SetText("SENTRY")
	if len(popup.visible) != 1 || npcIncomingTotal(ui) != 175 {
		t.Fatal("search changed damage or failed to find NPC")
	}
	test.Tap(popup.all)
	if npcIncomingTotal(ui) != 0 || filteredTotal(ui) != 70 {
		t.Fatal("All failed to clear search-hidden sources")
	}
	row := popup.list.CreateItem().(*damageFilterRow)
	popup.list.UpdateItem(0, row)
	test.Tap(row)
	if npcIncomingTotal(ui) != 40 || !popup.all.Partial {
		t.Fatal("NPC checkbox failed")
	}
	window := fyne.CurrentApp().NewWindow("Restored NPC filters")
	defer window.Close()
	restored := newAnalyzerUI(window)
	restored.snapshot = ui.snapshot
	restored.updateRange()
	if npcIncomingTotal(restored) != 40 || filteredTotal(restored) != 70 || !restored.view.Start.Equal(start) {
		t.Fatal("NPC selection did not survive restart")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_NPC_CAPTURE"))
}

func TestLocalNPCSources(t *testing.T) {
	directory := os.Getenv("EVELA_TEST_LOGS")
	if directory == "" {
		t.Skip("set EVELA_TEST_LOGS to inspect real logs read-only")
	}
	index := &logIndex{}
	snapshot, err := index.refresh(context.Background(), directory, logLoadingCutoff(time.Now(), 30), nil)
	if err != nil {
		t.Fatal(err)
	}
	names, groups := map[string]bool{}, map[int]bool{}
	var npcHits, otherHits int
	for _, log := range snapshot.Logs {
		for _, contribution := range log.IncomingSources {
			source := log.Sources[contribution.Source]
			if source.NPC {
				names[source.Name] = true
				groups[source.GroupID] = true
				npcHits += contribution.Hits
			} else {
				otherHits += contribution.Hits
			}
		}
		var damage float64
		var hits int
		for _, point := range log.Points {
			damage += point.Incoming
			hits += point.IncomingHits
		}
		for _, point := range log.IncomingSources {
			damage -= point.Damage
			hits -= point.Hits
		}
		if damage < -0.01 || damage > 0.01 || hits != 0 {
			t.Fatalf("incoming sources disagree with totals for %s: damage=%f, hits=%d", log.Character, damage, hits)
		}
	}
	t.Logf("%d files; %d NPC names; %d groups; %d NPC hits; %d other/unresolved hits", snapshot.Files, len(names), len(groups), npcHits, otherHits)
	if npcHits == 0 {
		t.Fatal("no NPC damage resolved in local archive")
	}
	start, end := presetRange("Last 24 hours", snapshot)
	view := buildView(snapshot, start, end, 0)
	selected := map[string]bool{}
	var expectedDamage float64
	var expectedHits int
	for _, series := range view.activeSeries() {
		selected[series.Name] = true
		expectedDamage += series.TotalIncoming
		expectedHits += series.IncomingHits
	}
	for _, mode := range incomingGroupingModes[2:] {
		var damage float64
		var hits int
		groups := view.incomingSourceSeries(snapshot, selected, mode, 0)
		for _, series := range groups {
			damage += series.TotalIncoming
			hits += series.IncomingHits
		}
		if damage-expectedDamage < -0.01 || damage-expectedDamage > 0.01 || hits != expectedHits {
			t.Fatalf("%s changed real-log incoming totals: damage=%f (want %f), hits=%d (want %d)", mode, damage, expectedDamage, hits, expectedHits)
		}
		t.Logf("%s: %d series; %.0f damage; %d hits", mode, len(groups), damage, hits)
	}
}
