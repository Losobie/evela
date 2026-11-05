package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func TestLoggedDamageTargets(t *testing.T) {
	for _, c := range []struct{ message, name, typ string }{
		{"912 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates", "Grijawlva", "Phoenix Navy Issue"},
		{"12 to Pilot - Name[C](Hulk) - Ogre I - Hits", "Pilot - Name", "Hulk"},
		{"12 to Blood Raiders Stronghold - Ogre I - Hits", "Blood Raiders Stronghold", "♦ Stronghold"},
		{"12 to Mining base[C](Athanor) - Hits", "Mining base", "Athanor"},
		{"Your Ogre I misses Pilot - Name[C](Hulk) completely", "Pilot - Name", "Hulk"},
		{"You miss Pilot[C](Hulk) completely - Ogre I", "Pilot", "Hulk"},
		{"12 from Enemy[C](Hulk) - Ogre I - Hits", "", ""},
	} {
		t.Run(c.message, func(t *testing.T) {
			log, err := parseLog(strings.NewReader("Listener: Pilot A\n[ 2026.10.19 02:00:00 ] (combat) " + c.message))
			if err != nil || len(log.Hits) != 1 {
				t.Fatalf("parse: %v, %+v", err, log)
			}
			if hit := log.Hits[0]; hit.Target != c.name || hit.TargetType != c.typ {
				t.Fatalf("target: %+v", hit)
			}
		})
	}
}

func newTargetTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	ui, start := newDamageFilterTestUI(t)
	var input strings.Builder
	input.WriteString("Listener: Pilot A\n")
	for _, message := range []string{
		"912 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates",
		"88 to Grijawlva[SNOOO](Hulk) - Ogre I - Hits",
		"400 to Other[C](Phoenix Navy Issue) - Ogre I - Hits",
		"50 to Blood Raiders Stronghold - Ogre I - Hits",
		"Your Ogre I misses Grijawlva[SNOOO](Phoenix Navy Issue) completely",
		"77 from Enemy[C](Hulk) - Ogre I - Hits",
		"120 remote shield boosted to Grijawlva[SNOOO](Phoenix Navy Issue)",
		"80 remote shield boosted to Other[C](Hulk)",
		"50 remote shield boosted by Enemy[C](Hulk)",
	} {
		fmt.Fprintf(&input, "[ 2026.10.19 02:00:00 ] (combat) %s\n", message)
	}
	log, err := parseLog(strings.NewReader(input.String()))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A"}, Logs: []*parsedLog{log}, First: start, Last: start}
	ui.graphAverageWindow = 0
	ui.applyGraphRange(start, start.Add(2*time.Second))
	return ui, start
}

func TestTargetFiltersGroupingAndPersistence(t *testing.T) {
	ui, start := newTargetTestUI(t)
	if len(ui.scopeFilterOptions(graphOutgoing, filterTarget)) != 3 || len(ui.scopeFilterOptions(graphOutgoing, filterTargetType)) != 3 {
		t.Fatal("target options incomplete")
	}
	for _, mode := range []string{filterTarget, filterTargetType} {
		chooseGraphMenuItem(t, ui.graphFormatMenu(), mode)
		var total float64
		for _, series := range ui.tableSeries {
			total += series.TotalOutgoing
			if series.TotalIncoming != 0 {
				t.Fatal("incoming damage was attributed to an outgoing target")
			}
		}
		if total != 1450 {
			t.Fatalf("grouped total: %v", total)
		}
		wantName, wantTotal := "Grijawlva", float64(1000)
		if mode == filterTargetType {
			wantName, wantTotal = "Phoenix Navy Issue", 1312
		}
		found := false
		for _, series := range ui.tableSeries {
			if series.Name == wantName {
				found = true
				if series.TotalOutgoing != wantTotal {
					t.Fatal("target group has incorrect total")
				}
			}
		}
		if !found {
			t.Fatalf("missing target group %q", wantName)
		}
		if mode == filterTarget {
			for _, series := range ui.outgoing.data.Series {
				if series.Name == "Grijawlva" && len(series.Hits) != 3 {
					t.Fatal("individual hits/miss lost target grouping")
				}
			}
		}
	}
	ui.setAllScopeFilters(graphOutgoing, filterTarget, false)
	ui.setScopeFilter(graphOutgoing, filterTarget, "name:Grijawlva", true)
	ui.setAllScopeFilters(graphOutgoing, filterTargetType, false)
	ui.setScopeFilter(graphOutgoing, filterTargetType, "name:Phoenix Navy Issue", true)
	if filteredTotal(ui) != 912 || len(ui.analysisSnapshot().Logs[0].Hits) != 3 {
		t.Fatal("target filters failed to intersect, or filtered incoming hits")
	}
	if ui.incoming.data.Series[0].Hits[0].Damage != 77 {
		t.Fatal("outgoing target filter affected incoming damage")
	}
	ui.graphAverageWindow = 10 * time.Second
	ui.applyGraphRange(start, start.Add(10*time.Second))
	if len(ui.outgoing.data.Series) != 1 || math.Abs(ui.outgoing.data.Series[0].Values[0]-91.2) > 1e-9 || filteredTotal(ui) != 912 {
		t.Fatal("averaging lost target attribution")
	}
	ui.setFilterVisible(graphOutgoing, filterTargetType, false)
	if filteredTotal(ui) != 1000 {
		t.Fatal("hidden target type filter still applied")
	}
	ui.setFilterVisible(graphOutgoing, filterTarget, false)
	ui.setFilterVisible(graphOutgoing, filterTargetType, true)
	ui.setAllScopeFilters(graphOutgoing, filterTargetType, false)
	ui.setScopeFilter(graphOutgoing, filterTargetType, "unknown", true)
	if filteredTotal(ui) != 0 {
		t.Fatal("catalog-resolved target remained under Unknown target type")
	}
	ui.setScopeFilter(graphOutgoing, filterTargetType, "unknown", false)
	ui.setScopeFilter(graphOutgoing, filterTargetType, "name:♦ Stronghold", true)
	if filteredTotal(ui) != 50 {
		t.Fatal("catalog target type did not retain Stronghold damage")
	}
	ui.setFilterVisible(graphOutgoing, filterTargetType, false)
	ui.setFilterVisible(graphOutgoing, filterTarget, true)
	window := fyne.CurrentApp().NewWindow("Restored targets")
	defer window.Close()
	restored := newAnalyzerUI(window)
	if restored.groupSelect.Selected != filterTargetType || restored.visibleFilters[graphOutgoing][filterTargetType] || !restored.storedScopeFilter(graphOutgoing, filterTarget).selected("name:Grijawlva") || restored.storedScopeFilter(graphOutgoing, filterTarget).selected("name:Other") {
		t.Fatal("target grouping/filter preferences did not persist")
	}
}

func TestLogisticsTargetsAndCatalogScope(t *testing.T) {
	ui, _ := newTargetTestUI(t)
	for _, mode := range []string{filterTarget, filterTargetType} {
		chooseGraphMenuItem(t, ui.eventGroupingMenu(graphOutgoingLogistics), mode)
		if len(ui.outgoingLogistics.data.Series) != 2 {
			t.Fatal("logistics target grouping missing")
		}
	}
	ui.setAllScopeFilters(graphOutgoingLogistics, filterTargetType, false)
	ui.setScopeFilter(graphOutgoingLogistics, filterTargetType, "name:Phoenix Navy Issue", true)
	if len(ui.outgoingLogistics.data.Series) != 1 || ui.outgoingLogistics.data.Series[0].Hits[0].Damage != 120 || ui.incomingLogistics.data.Series[0].Hits[0].Damage != 50 || filteredTotal(ui) != 1450 {
		t.Fatal("logistics target filter leaked or failed")
	}
	for _, scope := range []string{globalFilterScope, graphOutgoing, graphIncoming, graphOutgoingLogistics, graphIncomingLogistics, graphIncomingEWar, graphMining} {
		want := scope == graphOutgoing || scope == graphOutgoingLogistics
		for _, kind := range []string{filterTarget, filterTargetType} {
			if slices.Contains(availableFilters(scope), kind) != want {
				t.Fatalf("inappropriate %s filter in %s", kind, scope)
			}
			if scope != globalFilterScope && scope != graphOutgoing && slices.Contains(eventGroupingOptions(scope), kind) != (scope == graphOutgoingLogistics) {
				t.Fatalf("inappropriate %s grouping in %s", kind, scope)
			}
		}
	}
}

func TestTargetFiltersStructureMarkers(t *testing.T) {
	ui, _ := newTargetTestUI(t)
	log := ui.snapshot.Logs[0]
	log.StructureEvents = []structureEvent{{Second: ui.snapshot.First.Unix(), Target: "Base[C] (Fortizar)", Type: "Fortizar"}}
	ui.setAllScopeFilters(graphOutgoing, filterTarget, false)
	ui.setScopeFilter(graphOutgoing, filterTarget, "name:Base", true)
	if len(ui.analysisSnapshot().Logs[0].StructureEvents) != 1 {
		t.Fatal("matching structure target marker removed")
	}
	ui.setScopeFilter(graphOutgoing, filterTarget, "name:Base", false)
	if len(ui.analysisSnapshot().Logs[0].StructureEvents) != 0 {
		t.Fatal("excluded target marker retained")
	}
}
