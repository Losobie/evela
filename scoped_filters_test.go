package main

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestScopedFiltersAddRemovePersistAndIntersect(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("Scoped filters")
	defer window.Close()
	ui := newAnalyzerUI(window)
	for scope, kinds := range ui.visibleFilters {
		for kind, visible := range kinds {
			if visible {
				t.Fatalf("new installation shows %s/%s", scope, kind)
			}
		}
	}
	input := "Listener: Pilot\n" + strings.Join([]string{
		"[ 2026.10.19 02:00:00 ] (combat) 100 to Enemy - Laser - Hits",
		"[ 2026.10.19 02:00:00 ] (combat) 200 to Enemy - Ogre I - Penetrates",
		"[ 2026.10.19 02:00:00 ] (combat) 300 from Enemy[C](Hulk) - Laser - Hits",
		"[ 2026.10.19 02:00:00 ] (combat) 400 from Enemy[C](Hulk) - Ogre I - Smashes",
		"[ 2026.10.19 02:00:00 ] (combat) 50 remote armor repaired to Enemy",
		"[ 2026.10.19 02:00:00 ] (combat) 20 remote shield boosted to Enemy",
		"[ 2026.10.19 02:00:00 ] (combat) 10 GJ energy neutralized Enemy - Neut",
		"[ 2026.10.19 02:00:00 ] (combat) Warp scramble attempt from Enemy to you!",
		"[ 2026.10.19 02:00:00 ] (mining) You mined 5 units of Veldspar with a lost residue of 2 units",
	}, "\n")
	log, err := parseLog(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot"}, Logs: []*parsedLog{log}, First: start, Last: start}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(2*time.Second))
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1200, 800))
	window.Show()
	total := func() (float64, float64) {
		var in, out float64
		for _, series := range ui.view.activeSeries() {
			in += series.TotalIncoming
			out += series.TotalOutgoing
		}
		return in, out
	}
	assertTotal := func(wantIn, wantOut float64) {
		t.Helper()
		in, out := total()
		if in != wantIn || out != wantOut {
			t.Fatalf("totals %v/%v want %v/%v", in, out, wantIn, wantOut)
		}
	}
	assertTotal(700, 300)
	chooseGraphMenuItem(t, ui.addFilterMenu(graphOutgoing), filterWeaponType)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Ogre I", true)
	assertTotal(700, 200)
	if ui.visibleFilters[graphIncoming][filterWeaponType] {
		t.Fatal("outgoing visibility leaked to incoming")
	}
	for _, item := range ui.addFilterMenu(graphOutgoing).Items {
		if item.Label == filterWeaponType {
			t.Fatal("Add filter offers an already visible filter")
		}
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, false)
	assertTotal(700, 300)
	if ui.scopedFilterButtons[graphOutgoing][filterWeaponType] != nil {
		t.Fatal("removed filter button is still visible")
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, true)
	assertTotal(700, 200)
	ui.setFilterVisible(graphIncoming, filterCharacters, true)
	ui.setAllScopeFilters(graphIncoming, filterCharacters, false)
	assertTotal(0, 200)
	ui.setFilterVisible(graphIncoming, filterCharacters, false)
	assertTotal(700, 200)
	ui.setFilterVisible(globalFilterScope, filterShipType, true)
	ui.setAllScopeFilters(globalFilterScope, filterShipType, false)
	assertTotal(0, 0)
	if len(ui.outgoingLogistics.data.Series) != 0 || len(ui.ewar.data.Series) != 0 || len(ui.mining.data.Series) != 0 {
		t.Fatal("global filter failed to intersect event graphs")
	}
	ui.setFilterVisible(globalFilterScope, filterShipType, false)
	assertTotal(700, 200)
	ui.setFilterVisible(graphOutgoingLogistics, filterLogisticsType, true)
	ui.setAllScopeFilters(graphOutgoingLogistics, filterLogisticsType, false)
	if len(ui.outgoingLogistics.data.Series) != 0 {
		t.Fatal("local logistics filter ignored")
	}
	ui.setFilterVisible(graphOutgoingLogistics, filterLogisticsType, false)
	if len(ui.outgoingLogistics.data.Series) != 2 {
		t.Fatal("hidden logistics filter still active")
	}
	ui.setFilterVisible(graphMining, filterMiningMessage, true)
	ui.setAllScopeFilters(graphMining, filterMiningMessage, false)
	if len(ui.mining.data.Series) != 0 {
		t.Fatal("local mining filter ignored")
	}
	ui.setFilterVisible(graphMining, filterMiningMessage, false)
	if len(ui.mining.data.Series) != 2 {
		t.Fatal("hidden mining filter still active")
	}
	ui.setFilterVisible(graphIncomingEWar, filterEffectType, true)
	ui.setAllScopeFilters(graphIncomingEWar, filterEffectType, false)
	ui.setScopeFilter(graphIncomingEWar, filterEffectType, "Warp scramble", true)
	if len(ui.ewar.data.Series) != 1 || ui.ewar.data.Series[0].Name != "Warp scramble" {
		t.Fatal("E-War effect filter ignored")
	}
	assertTotal(700, 200)
	// Both visibility and retained selections survive a new UI instance.
	restoredWindow := app.NewWindow("Restored filters")
	defer restoredWindow.Close()
	restored := newAnalyzerUI(restoredWindow)
	restored.snapshot = ui.snapshot
	restored.graphAverageWindow = 0
	restored.applyGraphRange(start, start.Add(2*time.Second))
	restored.rebuildView()
	if !reflect.DeepEqual(restored.visibleFilters, ui.visibleFilters) || restored.view.activeSeries()[0].TotalOutgoing != 200 || len(restored.mining.data.Series) != 2 {
		t.Fatal("scoped filter persistence failed")
	}
	// Captures both filter bars and independent per-graph filters for visual QA.
	captureWindowPreview(t, window, os.Getenv("EVELA_SCOPED_FILTER_CAPTURE"))
}

func TestGraphFilterAndGroupingCatalogs(t *testing.T) {
	for _, graph := range graphVisibilityOptions {
		kinds := availableFilters(graph.id)
		for _, kind := range sharedDamageFilterKinds {
			if !slices.Contains(kinds, kind) {
				t.Fatalf("%s missing %s", graph.id, kind)
			}
		}
		damage := graph.id == graphIncoming || graph.id == graphOutgoing
		if slices.Contains(kinds, filterWeaponType) != damage || slices.Contains(kinds, filterWeaponStrength) != damage {
			t.Fatal("weapon filters offered for unrelated data")
		}
		if slices.Contains(kinds, filterNPCNames) != (graph.id == graphIncoming) {
			t.Fatal("NPC filter offered for unrelated data")
		}
		if slices.Contains(kinds, filterLogisticsType) != (graph.id == graphIncomingLogistics || graph.id == graphOutgoingLogistics) {
			t.Fatal("repair Type filter offered for unrelated data")
		}
	}
	if slices.Contains(availableFilters(globalFilterScope), filterWeaponType) {
		t.Fatal("damage-only filter offered globally")
	}
	for _, id := range []string{graphMining, graphIncomingEWar, graphIncomingLogistics, graphOutgoingLogistics} {
		modes := eventGroupingOptions(id)
		if !slices.Contains(modes, filterSystem) || !slices.Contains(modes, eventByClass) {
			t.Fatalf("%s missing meaningful history grouping", id)
		}
		if slices.Contains(modes, filterWeaponType) || slices.Contains(modes, incomingByNPC) {
			t.Fatal("unrelated grouping offered")
		}
	}
	if !slices.Contains(eventGroupingOptions(graphMining), eventByOreName) || slices.Contains(eventGroupingOptions(graphMining), filterMiningMessage) {
		t.Fatal("mining must support ore names without redundant message grouping")
	}
}

func TestHiddenLocalSystemFilterMigratesNames(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("System migration")
	defer window.Close()
	ui := newAnalyzerUI(window)
	ui.storedScopeFilter(graphMining, filterSystem).set("system-name:jita", false)
	ui.migrateSystemFilterNames(locationResolver{systems: map[string]int64{"jita": 30000142}})
	if ui.storedScopeFilter(graphMining, filterSystem).selected("system:30000142") || ui.visibleFilters[graphMining][filterSystem] {
		t.Fatal("migration lost hidden local selection or activated it")
	}
}
