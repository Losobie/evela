package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestDamageFilterPopupsStayInsideWindow(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	assertVisible := func() {
		t.Helper()
		popup := ui.filterPopup
		bounds := ui.window.Canvas().Size()
		position, size := popup.popup.Position(), popup.popup.Size()
		if position.X < 0 || position.Y < 0 || position.X+size.Width > bounds.Width || position.Y+size.Height > bounds.Height {
			t.Fatalf("%s popup outside %v canvas: position=%v size=%v", popup.kind, bounds, position, size)
		}
		footer := fyne.CurrentApp().Driver().AbsolutePositionForObject(popup.done)
		if footer.Y < position.Y || footer.Y+popup.done.Size().Height > bounds.Height || popup.list.Size().Height <= 0 {
			t.Fatalf("%s footer or scrollable list is clipped", popup.kind)
		}
	}
	for _, kind := range damageFilterKinds {
		ui.window.Resize(fyne.NewSize(1440, 960))
		ui.filterOptions[kind] = nil
		for i := 0; i < 80; i++ {
			ui.filterOptions[kind] = append(ui.filterOptions[kind], damageFilterOption{Key: fmt.Sprintf("option:%d", i), Label: fmt.Sprintf("Option %02d", i)})
		}
		test.Tap(ui.filterButtons[kind])
		assertVisible()
		popup := ui.filterPopup
		ui.window.Resize(fyne.NewSize(1200, 800))
		ui.window.Canvas().Capture() // Apply overlay layout after resizing.
		assertVisible()
		popup.list.ScrollToBottom()
		if kind == filterNPCTypes {
			captureWindowPreview(t, ui.window, os.Getenv("EVELA_FILTER_BOUNDS_CAPTURE"))
		}
		popup.search.SetText("no matching option")
		assertVisible()
		test.Tap(popup.done)
		if popup.popup.Visible() {
			t.Fatal("Done did not close the popup")
		}
	}
}

func newDamageFilterTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	application := test.NewTempApp(t)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Searchable damage filters")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	enableLegacyTestFilters(ui)
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Pilot A", "Pilot B", "Pilot C"}, First: start, Last: start.Add(time.Minute), Logs: []*parsedLog{
		{Character: "Pilot A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 100, Incoming: 10}, {Second: start.Unix() + 60, Outgoing: 200, Incoming: 20}}, StructureEvents: []structureEvent{{Second: start.Unix() + 1, Target: "First target"}, {Second: start.Unix() + 61, Target: "Second target"}}},
		{Character: "Pilot B", Points: []damagePoint{{Second: start.Unix() + 20, Outgoing: 300, Incoming: 30}}},
		{Character: "Pilot C", Points: []damagePoint{{Second: start.Unix() + 10, Outgoing: 400, Incoming: 40}}},
	}}
	ui.shipHistory = shipHistory{
		1: {{CharacterID: 1, CharacterName: "Pilot A", TypeID: 1, TypeName: "Loki", GroupID: 963, ClassName: "Strategic Cruiser", FirstSeen: start, LastSeen: start.Add(5 * time.Second)},
			{CharacterID: 1, CharacterName: "Pilot A", TypeID: 2, TypeName: "Thrasher", GroupID: 420, ClassName: "Destroyer", FirstSeen: start.Add(time.Minute), LastSeen: start.Add(65 * time.Second)}},
		2: {{CharacterID: 2, CharacterName: "Pilot B", TypeID: 1, TypeName: "Loki", GroupID: 963, ClassName: "Strategic Cruiser", FirstSeen: start, LastSeen: start.Add(30 * time.Second)}},
	}
	ui.characterGroups = []characterGroup{{ID: "fleet", Name: "Fleet", Members: []string{"Pilot A", "Pilot B"}}, {ID: "scouts", Name: "Scouts", Members: []string{"Pilot A"}}}
	ui.applyGraphRange(start, start.Add(2*time.Minute))
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	return ui, start
}

func filteredTotal(ui *analyzerUI) float64 {
	var total float64
	for _, series := range ui.tableSeries {
		total += series.TotalOutgoing
	}
	return total
}

func TestDamageFiltersIntersectAtEventTimeAcrossGraphsAndTotals(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	if filteredTotal(ui) != 1000 {
		t.Fatal("unexpected unfiltered totals")
	}
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:1", true)
	if filteredTotal(ui) != 400 || len(ui.outgoing.data.Series) != 2 {
		t.Fatal("ship filter selected all damage for a character instead of the matching historical events")
	}
	ui.setAllDamageFilterOptions(filterShipClass, false)
	ui.setDamageFilterOption(filterShipClass, "class:963", true)
	ui.setAllDamageFilterOptions(filterCharacterGroup, false)
	ui.setDamageFilterOption(filterCharacterGroup, "group:fleet", true)
	ui.setDamageFilterOption(filterCharacters, "Pilot B", false)
	if filteredTotal(ui) != 100 || !strings.Contains(ui.summary.Text, "Outgoing 100 damage") {
		t.Fatal("filter categories did not intersect consistently in totals and summary")
	}
	ui.graphInvulnerabilityMarkers = true
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	if len(ui.outgoing.data.Markers) != 1 || ui.outgoing.data.Markers[0].Label != "First target" || ui.outgoing.data.Series[0].Values[60] != 0 {
		t.Fatal("markers or averaged damage leaked through the ship filter")
	}
	if len(ui.snapshot.Logs[0].Points) != 2 || len(ui.snapshot.Logs[0].StructureEvents) != 2 {
		t.Fatal("filtering modified the original logs")
	}
	if len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Values[0] != 10 || ui.outgoing.data.Series[0].Values[60] != 0 {
		t.Fatal("graphs ignored the shared event filters")
	}
	ui.groupSelect.SetSelected("Ship class history")
	// A type and class with no overlap must produce an empty result.
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:2", true)
	if filteredTotal(ui) != 0 || len(ui.outgoing.data.Series) != 0 {
		t.Fatal("different categories were combined with OR")
	}
	ui.setAllDamageFilterOptions(filterShipClass, false)
	ui.setDamageFilterOption(filterShipClass, "class:420", true)
	if filteredTotal(ui) != 200 || ui.outgoing.data.Series[0].Name != "Destroyer" || ui.incoming.data.Series[0].Name != "Pilot A" {
		t.Fatal("matching type/class filters did not restore the correct damage")
	}
	ui.applyGraphRange(start.Add(55*time.Second), start.Add(70*time.Second))
	ui.undoGraphRange()
	if filteredTotal(ui) != 200 || !ui.view.Start.Equal(start) {
		t.Fatal("range undo reset the active filters")
	}
}

func TestDamageFilterDropdownSearchAllAndCheckboxes(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	for _, kind := range damageFilterKinds {
		if ui.filterButtons[kind] == nil {
			t.Fatalf("no button for %s", kind)
		}
	}
	test.Tap(ui.filterButtons[filterShipType])
	popup := ui.filterPopup
	if popup == nil || !popup.popup.Visible() || len(popup.visible) != 3 || !popup.all.Checked {
		t.Fatal("filter dropdown did not open with search, All and available options")
	}
	popup.search.SetText("LOK")
	if len(popup.visible) != 1 || popup.visible[0].Label != "Loki" || filteredTotal(ui) != 1000 {
		t.Fatal("search changed the data or failed to match case-insensitively")
	}
	test.Tap(popup.all)
	if filteredTotal(ui) != 0 || popup.all.Checked || !strings.Contains(ui.filterButtons[filterShipType].Text, "None") {
		t.Fatal("All did not unselect search-hidden options")
	}
	test.Tap(popup.all)
	if filteredTotal(ui) != 1000 || !popup.all.Checked {
		t.Fatal("All did not restore all options")
	}
	row := popup.list.CreateItem().(*damageFilterRow)
	popup.list.UpdateItem(0, row)
	test.Tap(row)
	if filteredTotal(ui) != 600 || !popup.all.Partial || ui.damageFilterSelected(filterShipType, "type:1") {
		t.Fatal("row checkbox did not filter immediately or show partial All state")
	}
	popup.search.SetText("not a ship")
	if len(popup.visible) != 0 || filteredTotal(ui) != 600 {
		t.Fatal("an empty search changed selection")
	}
	popup.search.SetText("")
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_FILTER_CAPTURE"))
	ui.openDamageFilter(filterCharacters)
	popup = ui.filterPopup
	test.Tap(popup.all)
	if len(ui.outgoing.data.Series) != 0 || filteredTotal(ui) != 0 {
		t.Fatal("Characters All did not clear both graph and totals")
	}
	test.Tap(popup.all)
	if filteredTotal(ui) != 600 {
		t.Fatal("Characters All reset another category's selections")
	}
	ui.window.Resize(fyne.NewSize(1200, 800))
	ui.rebuildView()
	if popup.search.Text != "" || !popup.popup.Visible() {
		t.Fatal("refresh unexpectedly dismissed the filter")
	}
}

func TestDamageFilterPersistenceAndGroupOverlap(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	ui.setAllDamageFilterOptions(filterCharacterGroup, false)
	ui.setDamageFilterOption(filterCharacterGroup, "group:scouts", true)
	ui.groupSelect.SetSelected(characterGroupsMode)
	if len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Name != "Scouts" || ui.outgoing.data.Series[0].MemberCount != 1 {
		t.Fatal("overlapping memberships reintroduced an unchecked group")
	}
	ui.setDamageFilterOption(filterCharacters, "Pilot A", false)
	other := fyne.CurrentApp().NewWindow("Restored filters")
	defer other.Close()
	restored := newAnalyzerUI(other)
	restored.snapshot, restored.shipHistory, restored.characterGroups = ui.snapshot, ui.shipHistory, ui.characterGroups
	restored.applyGraphRange(start, start.Add(2*time.Minute))
	if restored.damageFilterSelected(filterCharacters, "Pilot A") || !restored.damageFilterSelected(filterCharacterGroup, "group:scouts") || restored.damageFilterSelected(filterCharacterGroup, "group:fleet") || filteredTotal(restored) != 0 {
		t.Fatal("saved filter choices did not survive restart")
	}
	ui.setDamageFilterOption(filterCharacters, "Pilot A", true)
	ui.characterGroups[1].Name = "Renamed scouts"
	ui.rebuildView()
	if ui.filterOptions[filterCharacterGroup][1].Label != "Renamed scouts" || !ui.damageFilterSelected(filterCharacterGroup, "group:scouts") {
		t.Fatal("renaming a group lost its stable filter selection")
	}
	ui.deleteCharacterGroup("scouts")
	if filteredTotal(ui) != 0 {
		t.Fatal("deleting the only selected group broadened the filter")
	}
}

// Existing filter interaction tests opt into the filters they exercise.
func enableLegacyTestFilters(ui *analyzerUI) {
	for scope := range ui.visibleFilters {
		for _, kind := range availableFilters(scope) {
			ui.visibleFilters[scope][kind] = true
		}
	}
}
