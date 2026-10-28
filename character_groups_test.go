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
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestCharacterGroupRowSelectsBeforeDoubleClickTimeout(t *testing.T) {
	test.NewTempApp(t)
	row := newCharacterGroupRow()
	var modifiers []fyne.KeyModifier
	moves := 0
	row.onSelect = func(modifier fyne.KeyModifier) { modifiers = append(modifiers, modifier) }
	row.onMove = func() { moves++ }
	event := &desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierControl}
	row.MouseDown(event)
	if !reflect.DeepEqual(modifiers, []fyne.KeyModifier{fyne.KeyModifierControl}) {
		t.Fatal("mouse-down did not select immediately with the held modifier")
	}
	row.MouseUp(event)
	// The eventual single-click callback must not toggle Ctrl selection again.
	row.Tapped(&fyne.PointEvent{})
	if len(modifiers) != 1 {
		t.Fatal("the delayed tap selected a second time")
	}
	row.MouseDown(event)
	row.MouseUp(event)
	row.MouseDown(event)
	row.MouseUp(event)
	row.DoubleTapped(&fyne.PointEvent{})
	if len(modifiers) != 3 || moves != 1 {
		t.Fatal("double-click did not preserve immediate selection and a single transfer")
	}
	row.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonSecondary})
	if len(modifiers) != 3 {
		t.Fatal("secondary mouse button changed the selection")
	}
	// Touch and programmatic taps still select without a desktop mouse event.
	row.Tapped(&fyne.PointEvent{})
	if len(modifiers) != 4 || modifiers[3] != 0 {
		t.Fatal("a tap without mouse-down did not select")
	}
}

func TestCharacterGroupMultiSelection(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Character group multi-selection")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{}
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		ui.snapshot.Characters = append(ui.snapshot.Characters, name)
		ui.snapshot.Logs = append(ui.snapshot.Logs, &parsedLog{Character: name, EntrySeconds: []int64{start.Unix()}})
	}
	ui.applyGraphRange(start, start.Add(time.Minute))
	ui.openCharacterGroupSettings()
	editor := ui.characterGroupEditor
	// Use the same row callbacks and mouse events as the desktop driver.
	click := func(list *widget.List, index int, modifiers fyne.KeyModifier) {
		t.Helper()
		row := list.CreateItem().(*characterGroupRow)
		list.UpdateItem(index, row)
		row.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: modifiers})
		row.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: modifiers})
		test.Tap(row)
	}
	assertSelected := func(list *widget.List, want ...string) {
		t.Helper()
		var got []string
		for i := 0; i < list.Length(); i++ {
			row := list.CreateItem().(*characterGroupRow)
			list.UpdateItem(i, row)
			if row.background.Visible() {
				got = append(got, row.label.Text)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("selected rows = %v, want %v", got, want)
		}
	}
	click(editor.leftList, 1, 0)
	click(editor.leftList, 3, fyne.KeyModifierControl)
	assertSelected(editor.leftList, "B", "D")
	click(editor.leftList, 1, fyne.KeyModifierControl)
	assertSelected(editor.leftList, "D")
	click(editor.leftList, 4, fyne.KeyModifierShift)
	assertSelected(editor.leftList, "B", "C", "D", "E")
	// Repeated Shift-clicks keep the original anchor and can shrink the range.
	click(editor.leftList, 2, fyne.KeyModifierShift)
	assertSelected(editor.leftList, "B", "C")
	click(editor.leftList, 4, fyne.KeyModifierControl|fyne.KeyModifierShift)
	assertSelected(editor.leftList, "B", "C", "D", "E")
	click(editor.leftList, 2, 0)
	assertSelected(editor.leftList, "C")
	click(editor.leftList, 0, fyne.KeyModifierShift)
	assertSelected(editor.leftList, "A", "B", "C")
	editor.refreshLists()
	assertSelected(editor.leftList, "A", "B", "C")
	test.Tap(editor.addButton)
	if !reflect.DeepEqual(editor.included, []string{"A", "B", "C"}) || !editor.addButton.Disabled() {
		t.Fatalf("batch add failed: %v", editor.included)
	}
	assertSelected(editor.leftList)
	click(editor.rightList, 0, 0)
	click(editor.rightList, 2, fyne.KeyModifierControl)
	assertSelected(editor.rightList, "A", "C")
	test.Tap(editor.removeButton)
	if !reflect.DeepEqual(editor.included, []string{"B"}) || !editor.removeButton.Disabled() {
		t.Fatalf("batch remove failed: %v", editor.included)
	}
	assertSelected(editor.rightList)
	// Losing eligible characters clears their selection and range anchor.
	click(editor.leftList, 0, 0)
	click(editor.leftList, 1, fyne.KeyModifierShift)
	click(editor.leftList, 2, fyne.KeyModifierControl)
	click(editor.leftList, 3, fyne.KeyModifierControl)
	ui.applyGraphRange(start.Add(time.Hour), start.Add(2*time.Hour))
	assertSelected(editor.leftList, "A", "C") // Removed members stay eligible in this session.
	if editor.leftSelection.anchor != "" {
		t.Fatal("an ineligible character remained the range anchor")
	}
	click(editor.leftList, 1, fyne.KeyModifierControl)
	click(editor.leftList, 0, fyne.KeyModifierControl)
	assertSelected(editor.leftList)
	if !editor.addButton.Disabled() {
		t.Fatal("clearing the selection did not disable Add")
	}
	editor.loadDraft("")
	if editor.leftSelection.anchor != "" || editor.rightSelection.anchor != "" {
		t.Fatal("switching drafts retained range anchors")
	}
}

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
	application.Preferences().SetBool(graphAveragePreference, false)
	application.Preferences().SetBool(graphInvulnerabilityPreference, true)
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
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+min(float32(1), width*.025), 80, 0))
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, ungroupedCharacters+" · C: 30 damage") || !strings.Contains(ui.outgoing.hoverReadings[1].Text, "Fleet · B: 20 damage") {
		t.Fatal("named-group hit hover lost character attribution or damage")
	}
	ui.characterSearch.SetText("fleet")
	if got := characterFilterChecks(ui.characters); !reflect.DeepEqual(got, map[string]bool{"A": true, "B": true}) {
		t.Fatalf("legend could not filter by group name: %v", got)
	}
	ui.characterSearch.SetText("")
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	if ui.outgoing.data.Series[0].Values[0] != 3 || math.Abs(ui.incoming.data.Series[0].Values[0]-.1) > 1e-10 || ui.tableSeries[0].TotalOutgoing != 30 {
		t.Fatal("group averaging changed raw totals or combined rolling DPS incorrectly")
	}
	ui.groupSelect.SetSelected("Group: Scouts")
	if len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Name != "Scouts" || len(ui.tableSeries) != 1 || !strings.Contains(ui.summary.Text, "1 selected") {
		t.Fatal("individual group filter included other characters or groups")
	}
	ui.selectRangeCharacters(false)
	if ui.selected["B"] || !ui.selected["A"] || !ui.selected["C"] || len(ui.outgoing.data.Markers) != 0 || len(ui.outgoing.data.Series) != 0 || len(ui.tableSeries) != 0 {
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
