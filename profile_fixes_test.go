package main

import (
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2/widget"
)

func TestEventsAreDeferredUntilTabIsSelected(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	ui.snapshot.Logs[0].Events = []logEvent{{Second: start.Unix(), Message: "hello zero"}, {Second: start.Unix() + 1, Message: "hello one"}, {Second: start.Unix() + 2, Message: "hello two"}}
	ui.rebuildView()
	if !ui.eventsDirty || len(ui.events.rows) != 0 {
		t.Fatal("graph rebuild eagerly populated Events")
	}
	ui.applyGraphRange(start.Add(time.Second), start.Add(3*time.Second))
	if len(ui.events.rows) != 0 {
		t.Fatal("hidden Events updated during zoom")
	}
	ui.tabs.SelectIndex(2)
	if ui.eventsDirty || len(ui.events.rows) != 2 || ui.events.rows[0].Message != "hello one" {
		t.Fatal("opening Events did not apply the latest range")
	}
	ui.events.regex.SetChecked(true)
	ui.events.search.SetText("hello")
	ui.events.table.OnSelected(widget.TableCellID{Row: 0, Col: 0})
	ui.tabs.SelectIndex(0)
	ui.applyGraphRange(start.Add(2*time.Second), start.Add(3*time.Second))
	if !ui.eventsDirty || len(ui.events.rows) != 2 {
		t.Fatal("hidden Events did not defer processing")
	}
	ui.tabs.SelectIndex(2)
	if len(ui.events.rows) != 1 || ui.events.rows[0].Message != "hello two" || !ui.events.descending || !ui.events.regex.Checked || ui.events.search.Text != "hello" {
		t.Fatal("returning to Events lost current data or search/sort")
	}
	ui.tabs.SelectIndex(0)
	snapshot := *ui.snapshot
	ui.snapshot = &snapshot
	ui.rebuildView()
	if ui.events.snapshot != nil {
		t.Fatal("hidden Events retained an obsolete full archive")
	}
}

func TestMetadataCacheSurvivesRangeChangesAndRefreshesWithHistory(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	before := ui.graphMetadata()
	first := before.attributes("Pilot A", start.Unix())
	if first.hull.Label != "Loki" {
		t.Fatal("initial ship resolution failed")
	}
	ui.applyGraphRange(start, start.Add(90*time.Second))
	if ui.graphMetadata() != before {
		t.Fatal("zoom rebuilt the immutable metadata index")
	}
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, true)
	if ui.graphMetadata() != before {
		t.Fatal("filter change rebuilt the metadata index")
	}
	ui.shipHistory[1][0].TypeName = "Updated hull"
	ui.rebuildView()
	if ui.graphMetadata() == before || ui.graphMetadata().attributes("Pilot A", start.Unix()).hull.Label != "Updated hull" {
		t.Fatal("metadata refresh retained stale ship data")
	}
}

func TestFilteredSnapshotSharesUnchangedDataAndDoesNotMutateArchive(t *testing.T) {
	ui, start := newIncomingGroupingTestUI(t)
	original := ui.snapshot.Logs[0]
	original.Logistics = []logisticsEvent{{Second: start.Unix(), Kind: "Armor repair", Amount: 42}}
	original.Mining = []miningEvent{{Second: start.Unix(), Ore: "Veldspar", Kind: miningMined, Amount: 7}}
	ui.rebuildView()
	ui.setFilterVisible(graphOutgoing, filterWeaponType, true)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Laser", true)
	copyLog := ui.analysisSnapshot().Logs[0]
	if !sameRecords(copyLog.Hits, original.Hits) || !sameRecords(copyLog.Points, original.Points) || !sameRecords(copyLog.Logistics, original.Logistics) || !sameRecords(copyLog.Mining, original.Mining) {
		t.Fatal("non-excluding filter copied unchanged records")
	}
	originalHits := append([]damageHit(nil), original.Hits...)
	originalPoints := append([]damagePoint(nil), original.Points...)
	ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Laser", false)
	copyLog = ui.analysisSnapshot().Logs[0]
	if sameRecords(copyLog.Hits, original.Hits) || !sameRecords(copyLog.Logistics, original.Logistics) {
		t.Fatal("filter did not isolate changed records from unchanged event families")
	}
	if !reflect.DeepEqual(originalHits, original.Hits) || !reflect.DeepEqual(originalPoints, original.Points) {
		t.Fatal("filter modified shared archive data")
	}
}

func TestSelectRecordsAllocatesOnlyAfterFirstExclusion(t *testing.T) {
	records := []int{1, 2, 3, 4}
	if !sameRecords(records, selectRecords(records, func(int) bool { return true })) {
		t.Fatal("matching records were copied")
	}
	selected := selectRecords(records, func(value int) bool { return value%2 == 0 })
	if !reflect.DeepEqual(selected, []int{2, 4}) {
		t.Fatal("filter result changed")
	}
	selected[0] = 99
	if records[1] != 2 {
		t.Fatal("filtered output overwrote archive storage")
	}
}
