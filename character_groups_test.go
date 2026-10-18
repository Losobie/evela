package main

import (
	"image/png"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestGroupCandidatesUseActualLogEntriesInWindow(t *testing.T) {
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{}
	for _, content := range []string{
		"Listener: Damage\n[ 2026.10.18 02:00:00 ] (combat) 10 to Enemy - Hits\n",
		"Listener: Notification\n[ 2026.10.18 02:00:05 ] (notify) Warping to destination\n[ 2026.10.18 02:00:05 ] (notify) Something else\n",
		"Listener: Boundary\n[ 2026.10.18 02:00:10 ] (notify) Outside\n",
		"Listener: Gap\n[ 2026.10.18 01:59:59 ] (notify) Before\n[ 2026.10.18 02:00:11 ] (notify) After\n",
		"Listener: Invalid\n[ bad timestamp ] (notify) No\n",
	} {
		parsed, err := parseLog(strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Logs = append(snapshot.Logs, parsed)
	}
	got := sortedCharacterNames(charactersWithLogEntries(snapshot, start, start.Add(10*time.Second)))
	if !reflect.DeepEqual(got, []string{"Damage", "Notification"}) || len(snapshot.Logs[1].EntrySeconds) != 1 {
		t.Fatalf("candidates ignored non-damage entries, gaps or exclusive bounds: %v", got)
	}
}

func TestCharacterGroupEditorTransfersAndSessionEligibility(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Character group editor")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"A", "B", "Outside"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 10}}},
		{Character: "B", EntrySeconds: []int64{start.Unix() + 1}},
		{Character: "Outside", EntrySeconds: []int64{start.Unix() + 20}},
	}}
	ui.selected["A"] = true
	ui.applyGraphRange(start, start.Add(10*time.Second))
	existing, err := ui.saveCharacterGroup(characterGroup{Name: "Existing", Members: []string{"A", "Old Pilot"}})
	if err != nil {
		t.Fatal(err)
	}
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	for _, item := range window.MainMenu().Items[0].Items {
		if item.Label == "Character groups…" {
			item.Action()
		}
	}
	editor := ui.characterGroupEditor
	if editor == nil {
		t.Fatal("Settings did not open the character group editor")
	}
	editor.choice.SetSelected("Edit: Existing")
	if !reflect.DeepEqual(editor.available, []string{"B"}) || !reflect.DeepEqual(editor.included, []string{"A", "Old Pilot"}) {
		t.Fatalf("editor did not separate eligible characters from all existing members: %v / %v", editor.available, editor.included)
	}
	findRow := func(list *widget.List, name string) *characterGroupRow {
		t.Helper()
		window.Canvas().Capture()
		var found *characterGroupRow
		seen := map[fyne.CanvasObject]bool{}
		var walk func(fyne.CanvasObject)
		walk = func(object fyne.CanvasObject) {
			if object == nil || seen[object] {
				return
			}
			seen[object] = true
			if row, ok := object.(*characterGroupRow); ok && row.label.Text == name {
				found = row
			}
			switch value := object.(type) {
			case *fyne.Container:
				for _, child := range value.Objects {
					walk(child)
				}
			case fyne.Widget:
				for _, child := range test.WidgetRenderer(value).Objects() {
					walk(child)
				}
			}
		}
		walk(list)
		if found == nil {
			t.Fatalf("no visible row for %q", name)
		}
		return found
	}
	// Native single-click routing must select the row for the arrow controls.
	row := findRow(editor.rightList, "Old Pilot")
	position := application.Driver().AbsolutePositionForObject(row).Add(fyne.NewPos(8, 8))
	test.TapCanvas(window.Canvas(), position)
	if editor.removeButton.Disabled() {
		t.Fatal("clicking a character did not enable the removal arrow")
	}
	test.Tap(editor.removeButton)
	if !reflect.DeepEqual(editor.available, []string{"B", "Old Pilot"}) {
		t.Fatal("removed out-of-window member did not become available")
	}
	editor.choice.SetSelected("New group…")
	if !reflect.DeepEqual(editor.available, []string{"A", "B", "Old Pilot"}) {
		t.Fatal("switching drafts lost the removed-character session pool")
	}
	test.DoubleTap(findRow(editor.leftList, "B"))
	if !reflect.DeepEqual(editor.included, []string{"B"}) {
		t.Fatal("double-click did not add the character")
	}
	row = findRow(editor.leftList, "Old Pilot")
	test.Tap(row)
	test.Tap(editor.addButton)
	if !reflect.DeepEqual(editor.included, []string{"B", "Old Pilot"}) {
		t.Fatal("add arrow failed for a removed inactive member")
	}
	test.DoubleTap(findRow(editor.rightList, "B"))
	if !reflect.DeepEqual(editor.included, []string{"Old Pilot"}) {
		t.Fatal("double-click did not remove the character")
	}
	editor.add("Outside")
	if len(editor.included) != 1 {
		t.Fatal("an ineligible character could be added")
	}
	editor.name.SetText("Fleet")
	test.Tap(editor.saveButton)
	if len(ui.characterGroups) != 2 || editor.id == "" || !strings.Contains(editor.status.Text, "Saved") {
		t.Fatal("new named group did not save")
	}
	if path := os.Getenv("EVELA_CHARACTER_GROUPS_CAPTURE"); path != "" {
		editor.choice.SetSelected("Edit: Existing")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, window.Canvas().Capture())
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	editor.loadDraft(existing.ID)
	editor.remove("Old Pilot")
	editor.dialog.Hide() // Unsaved draft edits must not modify saved membership.
	ui.openCharacterGroupSettings()
	editor = ui.characterGroupEditor
	editor.loadDraft(existing.ID)
	if !reflect.DeepEqual(editor.included, []string{"A", "Old Pilot"}) || !reflect.DeepEqual(editor.available, []string{"B"}) {
		t.Fatal("closing saved draft changes or retained the removed pool across sessions")
	}
	// Live refreshes update eligibility while preserving existing members.
	ui.applyGraphRange(start.Add(15*time.Second), start.Add(25*time.Second))
	if !reflect.DeepEqual(editor.available, []string{"Outside"}) || !reflect.DeepEqual(editor.included, []string{"A", "Old Pilot"}) {
		t.Fatal("time-window refresh did not update the left panel independently of membership")
	}
}

func TestCharacterGroupGroupingFilteringPersistenceAndDeletion(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Character group filters")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"A", "B", "C"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 10, Incoming: 1, OutgoingHits: 1}}},
		{Character: "B", Points: []damagePoint{{Second: start.Unix(), Outgoing: 20, Incoming: 2, OutgoingHits: 1}}, StructureEvents: []structureEvent{{Second: start.Unix() + 2, Target: "Test (Astrahus)"}}},
		{Character: "C", Points: []damagePoint{{Second: start.Unix(), Outgoing: 30, Incoming: 3, OutgoingHits: 1}}},
	}}
	for _, name := range ui.snapshot.Characters {
		ui.selected[name] = true
	}
	ui.applyGraphRange(start, start.Add(10*time.Second))
	fleet, err := ui.saveCharacterGroup(characterGroup{Name: "Fleet", Members: []string{"a", "A", "B", "Inactive"}})
	if err != nil || len(fleet.Members) != 3 {
		t.Fatalf("group creation or case-insensitive member deduplication failed: %+v / %v", fleet, err)
	}
	scouts, err := ui.saveCharacterGroup(characterGroup{Name: "Scouts", Members: []string{"B"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "fleet", " FLEET ", ungroupedCharacters} {
		if _, err := ui.saveCharacterGroup(characterGroup{Name: invalid}); err == nil {
			t.Fatalf("invalid/duplicate group name %q was accepted", invalid)
		}
	}
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	ui.groupSelect.SetSelected(characterGroupsMode)
	if !reflect.DeepEqual(totalsNames(ui), []string{"Fleet", "Scouts", ungroupedCharacters}) || ui.tableSeries[0].TotalOutgoing != 30 || ui.tableSeries[1].TotalOutgoing != 20 || ui.tableSeries[2].TotalOutgoing != 30 {
		t.Fatal("named groups did not combine members or keep ungrouped characters")
	}
	if !strings.Contains(ui.summary.Text, "3 selected") || !strings.Contains(ui.summary.Text, "Outgoing 60 damage") {
		t.Fatal("shared members were counted twice in the summary")
	}
	width := ui.outgoing.Size().Width - chartPlotLeft - chartPlotRight
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+width*.025, 80, 0))
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, "Fleet (2): 30 DPS") {
		t.Fatal("named-group hover lost membership counts or damage")
	}
	ui.characterSearch.SetText("fleet")
	if got := characterFilterChecks(ui.characters); !reflect.DeepEqual(got, map[string]bool{"A": true, "B": true}) {
		t.Fatalf("legend could not filter by group name: %v", got)
	}
	ui.characterSearch.SetText("")
	ui.graphAverage10s, ui.graphLogarithmic = true, true
	ui.renderSelection()
	if ui.outgoing.data.Series[0].Values[0] != 3 || math.Abs(ui.incoming.data.Series[0].Values[0]-.3) > 1e-10 || ui.tableSeries[0].TotalOutgoing != 30 {
		t.Fatal("group averaging changed raw totals or combined rolling DPS incorrectly")
	}
	ui.groupSelect.SetSelected("Group: Scouts")
	if len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Name != "Scouts" || len(ui.tableSeries) != 1 || !strings.Contains(ui.summary.Text, "1 selected") {
		t.Fatal("individual group filter included other characters or groups")
	}
	ui.selectRangeCharacters(false)
	if ui.selected["B"] || !ui.selected["A"] || !ui.selected["C"] || len(ui.outgoing.data.Markers) != 0 || len(ui.outgoing.data.Series) != 0 || len(ui.tableSeries) != 1 {
		t.Fatal("group filter changed hidden selections or ignored marker/graph selection")
	}
	ui.selectRangeCharacters(true)
	if len(ui.outgoing.data.Markers) != 1 {
		t.Fatal("group selection lost a member's structure marker")
	}
	scouts.Name = "Wing"
	if _, err := ui.saveCharacterGroup(scouts); err != nil || ui.groupSelect.Selected != "Group: Wing" {
		t.Fatal("renaming the active group lost its filter")
	}
	otherWindow := application.NewWindow("Restored groups")
	defer otherWindow.Close()
	restored := newAnalyzerUI(otherWindow)
	if len(restored.characterGroups) != 2 || restored.groupSelect.Selected != "Group: Wing" || restored.selectedCharacterGroupID() != scouts.ID {
		t.Fatal("groups or the active group filter did not survive a new UI")
	}
	ui.deleteCharacterGroup(scouts.ID)
	if ui.groupSelect.Selected != "Characters" || len(ui.characterGroups) != 1 {
		t.Fatal("deleting the active group did not fall back to per-character mode")
	}
}
