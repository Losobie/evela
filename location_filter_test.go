package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestSystemFilterAtEventTimeWithAveragingMarkersAndPersistence(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	ui.locationHistory = locationHistory{
		1: {{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30000142, SystemName: "Jita", FirstSeen: start, LastSeen: start.Add(5 * time.Second)},
			{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30002187, SystemName: "Amarr", FirstSeen: start.Add(60 * time.Second), LastSeen: start.Add(65 * time.Second)}},
		2: {{CharacterID: 2, CharacterName: "Pilot B", SystemID: 30000142, SystemName: "Jita", FirstSeen: start, LastSeen: start.Add(30 * time.Second)}},
	}
	ui.graphInvulnerabilityMarkers = true
	ui.rebuildView()
	if len(ui.filterOptions[filterSystem]) != 3 {
		t.Fatalf("missing systems/unknown option: %+v", ui.filterOptions[filterSystem])
	}
	ui.setAllDamageFilterOptions(filterSystem, false)
	ui.setDamageFilterOption(filterSystem, "system:30000142", true)
	if filteredTotal(ui) != 400 || ui.outgoing.data.Series[0].Values[60] != 0 || len(ui.outgoing.data.Markers) != 1 {
		t.Fatal("system filter leaked damage, averages or markers from another location")
	}
	ui.groupSelect.SetSelected("Ship class history")
	if len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Name != "Strategic Cruiser" {
		t.Fatal("system filter failed with ship grouping")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:2", true)
	if filteredTotal(ui) != 0 {
		t.Fatal("System and ship filters did not intersect")
	}
	ui.setAllDamageFilterOptions(filterShipType, true)
	ui.setDamageFilterOption(filterSystem, "system:30002187", true)
	if filteredTotal(ui) != 600 {
		t.Fatal("selected systems did not combine with OR")
	}
	ui.setAllDamageFilterOptions(filterSystem, false)
	ui.setDamageFilterOption(filterSystem, "unknown", true)
	if filteredTotal(ui) != 400 {
		t.Fatal("unknown location assigned to a current system")
	}
	ui.openDamageFilter(filterSystem)
	ui.filterPopup.search.SetText("jit")
	if len(ui.filterPopup.visible) != 1 {
		t.Fatal("system search failed")
	}
	test.Tap(ui.filterPopup.all)
	if filteredTotal(ui) != 1000 {
		t.Fatal("System All did not include search-hidden options")
	}
	ui.setDamageFilterOption(filterSystem, "system:30000142", false)
	other := fyne.CurrentApp().NewWindow("Restored System filter")
	defer other.Close()
	restored := newAnalyzerUI(other)
	restored.snapshot, restored.shipHistory, restored.locationHistory = ui.snapshot, ui.shipHistory, ui.locationHistory
	restored.applyGraphRange(start, start.Add(2*time.Minute))
	restored.rebuildView()
	if restored.damageFilterSelected(filterSystem, "system:30000142") || filteredTotal(restored) != 600 {
		t.Fatalf("system selection did not survive restart: selected=%v total=%v filters=%+v groups=%q", restored.damageFilterSelected(filterSystem, "system:30000142"), filteredTotal(restored), restored.damageFilters, restored.groupSelect.Selected)
	}
	if len(ui.snapshot.Logs[0].Points) != 2 {
		t.Fatal("system filtering mutated original damage")
	}
}

func TestLocationHistoryViewer(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	ui.openLocationHistory()
	if ui.window.Canvas().Overlays().Top() == nil {
		t.Fatal("empty location viewer failed to open")
	}
	history := locationHistory{1: {{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30000142, FirstSeen: start, LastSeen: start, Observations: 1},
		{CharacterID: 1, CharacterName: "Pilot A", SystemID: 30002187, SystemName: "Amarr", StationID: 60008494, FirstSeen: start.Add(time.Minute), LastSeen: start.Add(time.Minute), Observations: 1}}}
	content := locationHistoryContent(history).(*fyne.Container)
	var table *widget.Table
	for _, object := range content.Objects {
		if candidate, ok := object.(*widget.Table); ok {
			table = candidate
		}
	}
	if table == nil {
		t.Fatal("missing location history table")
	}
	label := widget.NewLabel("")
	for col, want := range []string{"Pilot A", "Amarr", "30002187", "Station 60008494", "2026-10-19 02:01:00", "2026-10-19 02:01:00", "1"} {
		table.UpdateCell(widget.TableCellID{Row: 1, Col: col}, label)
		if label.Text != want {
			t.Fatalf("column %d: %q, want %q", col, label.Text, want)
		}
	}
	history[1][1].SystemName = "changed"
	table.UpdateCell(widget.TableCellID{Row: 1, Col: 1}, label)
	if label.Text != "Amarr" {
		t.Fatal("open location viewer changed after refresh")
	}
	table.UpdateCell(widget.TableCellID{Row: 2, Col: 1}, label)
	if label.Text != "System 30000142" {
		t.Fatal("unresolved system ID hidden")
	}
}
