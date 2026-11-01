package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRecordWindowBoundsAndSharedStorage(t *testing.T) {
	records := []damageHit{{Second: 1}, {Second: 10}, {Second: 10}, {Second: 11}, {Second: 20}}
	got := recordsInRange(records, 10, 20, func(hit damageHit) int64 { return hit.Second })
	if len(got) != 3 || &got[0] != &records[1] || got[2].Second != 11 {
		t.Fatalf("window includes wrong bounds or copies records: %+v", got)
	}
	for _, bounds := range [][2]int64{{-10, 0}, {21, 30}, {10, 10}} {
		if len(recordsInRange(records, bounds[0], bounds[1], func(hit damageHit) int64 { return hit.Second })) != 0 {
			t.Fatal("empty window retained records")
		}
	}
}

func TestWindowedGraphResultsMatchWholeArchive(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	var input strings.Builder
	input.WriteString("Listener: Logi\n")
	for _, offset := range []int64{-3600, -31, -30, -29, -10, -9, 0, 2, 7, 8, 3600} {
		timestamp := start.Add(time.Duration(offset) * time.Second).Format(logTimeLayout)
		for _, message := range []string{
			"(combat) 100 to Enemy - Laser - Hits",
			"(combat) 20 from Enemy[C](Hulk) - Drone - Smashes",
			"(combat) Enemy[C](Hulk) misses you completely",
			"(combat) 30 remote armor repaired to Enemy",
			"(combat) 40 remote capacitor transmitted by Enemy",
			"(combat) 10 GJ energy neutralized Enemy - Neutralizer",
			"(mining) You mined 5 units of Veldspar with a lost residue of 2 units",
		} {
			fmt.Fprintf(&input, "[ %s ] %s\n", timestamp, message)
		}
	}
	log, err := parseLog(strings.NewReader(input.String()))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot.Logs = append(ui.snapshot.Logs, log)
	ui.rebuildView()
	originalHits := append([]damageHit(nil), log.Hits...)
	charts := []*damageChart{ui.outgoing, ui.incoming, ui.outgoingLogistics, ui.incomingLogistics, ui.ewar, ui.mining}
	for _, mode := range []string{"Characters", "Ship history", filterWeaponType, filterSystem, characterGroupsMode} {
		ui.groupSelect.Selected = mode
		ui.incomingGrouping = incomingByShip
		for _, window := range []time.Duration{0, 10 * time.Second, 30 * time.Second} {
			ui.graphAverageWindow = window
			for _, restricted := range []bool{false, true} {
				ui.visibleFilters[graphOutgoing][filterWeaponType] = restricted
				selection := ui.storedScopeFilter(graphOutgoing, filterWeaponType)
				selection.Default = false
				selection.Values = map[string]bool{"name:Laser": true}
				ui.rebuildView()
				var optimized []plotData
				for _, chart := range charts {
					optimized = append(optimized, chart.data)
				}
				totals, summary := ui.tableSeries, ui.summary.Text
				// Reference path deliberately uses every record in the archive.
				ui.analysis = ui.filteredDamageSnapshot(ui.snapshot)
				ui.view = buildView(ui.analysisSnapshot(), ui.view.Start, ui.view.End, ui.view.Bucket)
				ui.renderSelection()
				for index, chart := range charts {
					if !reflect.DeepEqual(optimized[index], chart.data) {
						t.Fatalf("graph %d changed for mode %s average %v restricted=%v", index, mode, window, restricted)
					}
				}
				if !reflect.DeepEqual(totals, ui.tableSeries) || summary != ui.summary.Text {
					t.Fatal("window optimization changed totals or summary")
				}
			}
		}
	}
	if !reflect.DeepEqual(originalHits, log.Hits) {
		t.Fatal("window filtering mutated archive records")
	}
}

func TestFilterChangesReuseOptionsAndRangeChangesRefreshThem(t *testing.T) {
	ui, start := newIncomingGroupingTestUI(t)
	before := ui.scopeFilterOptions(graphOutgoing, filterWeaponType)
	if len(before) == 0 {
		t.Fatal("missing raw weapon options")
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, true)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	after := ui.scopeFilterOptions(graphOutgoing, filterWeaponType)
	if &before[0] != &after[0] {
		t.Fatal("filter selection rebuilt unchanged raw options")
	}
	ui.applyGraphRange(start.Add(time.Hour), start.Add(time.Hour+time.Second))
	if len(ui.scopeFilterOptions(graphOutgoing, filterWeaponType)) != 0 {
		t.Fatal("zoom retained options from old time range")
	}
	ui.undoGraphRange()
	if len(ui.scopeFilterOptions(graphOutgoing, filterWeaponType)) == 0 {
		t.Fatal("undo did not restore raw options")
	}
	// Imported snapshots and metadata refreshes take the full rebuild path.
	ui.snapshot.Logs = nil
	ui.rebuildView()
	if len(ui.scopeFilterOptions(graphOutgoing, filterWeaponType)) != 0 {
		t.Fatal("snapshot refresh retained stale options")
	}
}
