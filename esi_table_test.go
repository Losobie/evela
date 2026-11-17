package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestESITableRangesCopyFullValuesAndRefresh(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	checked := time.Date(2026, 11, 16, 12, 34, 56, 0, time.FixedZone("Pilot local", -5*3600))
	shipName := strings.Repeat("Long ship name ", 20)
	records := []shipRecord{
		{CharacterID: 1, CharacterName: "Pilot A", Scopes: []string{shipScope, locationScope, structureScope}, TypeName: "Loki", ClassName: "Strategic Cruiser", ShipName: shipName, CheckedAt: checked, Error: "Ship lookup failed", ClassError: "Class lookup failed", Location: locationRecord{SystemName: "Jita", StationID: 60003760, StationName: "Jita IV - Moon 4 - Caldari Navy Assembly Plant", CheckedAt: time.Now()}},
		{CharacterID: 2, CharacterName: "Pilot B", Scopes: []string{shipScope}},
	}
	ui.applyShipRecords(records)
	ui.tabs.SelectIndex(4)
	table := ui.esiConnections
	for col, heading := range []string{"Character", "Time Checked", "Ship", "Location", "Scopes", "ESI Status"} {
		cell := renderedSelectionCell(t, table, widget.TableCellID{Col: col})
		label := cell.content.(*widget.Label)
		if label.Text != heading || !label.TextStyle.Bold {
			t.Fatalf("incorrect heading %d: %q", col, label.Text)
		}
	}
	if ui.esiRows[0][1] != "2026-11-16 17:34:56" || ui.esiRows[1][1] != "Not checked" {
		t.Fatalf("checked times should be EVE time without timezone text: %v", ui.esiRows)
	}
	if ui.esiRows[0][2] != "Loki" || ui.esiRows[1][2] != "Ship unavailable" {
		t.Fatalf("ship cells should contain only the ship type: %v", ui.esiRows)
	}
	if ui.esiRows[0][3] != "Jita IV - Moon 4 - Caldari Navy Assembly Plant" || ui.esiRows[1][3] != "Location unavailable" {
		t.Fatalf("incorrect locations: %v", ui.esiRows)
	}
	if ui.esiRows[0][5] != "Ship: Ship lookup failed · Ship class: Class lookup failed" || ui.esiRows[1][5] != "Not checked" {
		t.Fatalf("status should belong to each character: %v", ui.esiRows)
	}
	if ui.esiRows[0][4] != "All" || ui.esiRows[1][4] != "Ship" {
		t.Fatalf("incorrect scope labels: %v", ui.esiRows)
	}
	first := renderedSelectionCell(t, table, widget.TableCellID{Row: 1, Col: 0})
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if selectionCopy(table) != "Pilot A" || ui.window.Canvas().Focused() != table {
		t.Fatal("ESI cell did not select immediately and receive clipboard focus")
	}
	last := renderedSelectionCell(t, table, widget.TableCellID{Row: 2, Col: 5})
	start := fyne.CurrentApp().Driver().AbsolutePositionForObject(first)
	end := fyne.CurrentApp().Driver().AbsolutePositionForObject(last).Add(fyne.NewPos(10, 10))
	test.MoveMouse(ui.window.Canvas(), end)
	first.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: end.Subtract(start)}})
	first.DragEnd()
	first.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	first.Tapped(&fyne.PointEvent{})
	want := strings.Join(ui.esiRows[0][:], "\t") + "\n" + strings.Join(ui.esiRows[1][:], "\t")
	if len(table.selected) != 12 || selectionCopy(table) != want {
		t.Fatalf("drag did not copy the full rectangular values: %q", selectionCopy(table))
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_ESI_TABLE_CAPTURE"))
	last.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierControl})
	if len(table.selected) != 11 {
		t.Fatal("Ctrl-click did not toggle a cell")
	}
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	last.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierShift})
	if len(table.selected) != 12 {
		t.Fatal("Shift-click did not extend the range")
	}
	ui.applyShipRecords(records[1:])
	if len(table.selected) != 0 {
		t.Fatal("refresh retained selection of replaced records")
	}
	ui.applyShipRecords(nil)
	if rows, cols := table.Length(); rows != 1 || cols != 6 {
		t.Fatalf("empty connections should show only the six headings: %dx%d", rows, cols)
	}
}

func TestESITableKeepsDockedNamesWhenLogsConfirmSystem(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	now := time.Now()
	ui.logLocations = logLocationHistory{"current": {CharacterName: "Pilot A", Start: now.Add(-time.Minute).Unix(), End: now.Add(time.Minute).Unix(), Events: []logLocationEvent{{Second: now.Add(-time.Minute).Unix(), To: "Jita"}}}}
	record := shipRecord{CharacterName: "Pilot A", Location: locationRecord{SystemName: "Jita", StationID: 60003760, StationName: "Assembly Plant", CheckedAt: now}}
	ui.applyShipRecords([]shipRecord{record})
	if ui.esiRows[0][3] != "Jita · Assembly Plant" {
		t.Fatalf("matching game logs hid a current station name: %s", ui.esiRows[0][3])
	}
	record.Location.SystemName = "Amarr"
	ui.applyShipRecords([]shipRecord{record})
	if strings.Contains(ui.esiRows[0][3], "Assembly Plant") || !strings.Contains(ui.esiRows[0][3], "Jita") {
		t.Fatal("conflicting game logs retained a station in another system")
	}
	record.Location.SystemName = "Jita"
	record.Location.CheckedAt = now.Add(-time.Hour)
	ui.applyShipRecords([]shipRecord{record})
	if strings.Contains(ui.esiRows[0][3], "Assembly Plant") {
		t.Fatal("game logs resurrected a stale docking location")
	}
}

func TestESILocationValuesUseNamesOrBareIDs(t *testing.T) {
	for _, tc := range []struct {
		location locationRecord
		want     string
	}{
		{locationRecord{SystemID: 30000142, NameError: "lookup failed"}, "30000142"},
		{locationRecord{StationID: 60003760, DockedNameError: "lookup failed"}, "60003760"},
		{locationRecord{StructureID: 100000000001, DockedNameError: "missing scope"}, "100000000001"},
		{locationRecord{SystemName: "Jita", StationID: 60003760, Error: "refresh failed", DockedNameError: "lookup failed"}, "Jita · 60003760"},
		{locationRecord{SystemID: 30000142, StructureID: 100000000001}, "30000142 · 100000000001"},
		{locationRecord{SystemName: "Jita", StructureID: 100000000001, StructureName: "Fortizar"}, "Jita · Fortizar"},
	} {
		if got := locationLabel(tc.location); got != tc.want {
			t.Errorf("location value = %q, want %q", got, tc.want)
		}
	}
}

func TestESITableFitsViewport(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("ESI table layout")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	for i := 0; i < 100; i++ {
		ui.esiRows = append(ui.esiRows, [6]string{"Character", "2026-11-16 17:34:56", strings.Repeat("Long ship ", 20), strings.Repeat("Long location ", 20)})
	}
	holder := ui.esiTableContent()
	window.SetContent(holder)
	window.Show()
	scroll := selectionTableScroll(t, ui.esiConnections)
	for _, width := range []float32{480, 1440, 900, 520} {
		holder.Resize(fyne.NewSize(width, 400))
		ui.esiConnections.ScrollTo(widget.TableCellID{Row: 100, Col: 5})
		contentWidth := scroll.Content.MinSize().Width
		if contentWidth > scroll.Size().Width+0.1 || contentWidth < scroll.Size().Width-0.1 || scroll.Offset.X > 0.1 || scroll.Offset.Y <= 0 {
			t.Fatalf("width %.0f: content %.1f, viewport %.1f, scroll %v", width, contentWidth, scroll.Size().Width, scroll.Offset)
		}
	}
}

func TestCharacterESIStatus(t *testing.T) {
	now := time.Now()
	if got := characterESIStatus(shipRecord{CheckedAt: now}); got != "OK" {
		t.Fatalf("successful character status = %q", got)
	}
	record := shipRecord{CheckedAt: now, Location: locationRecord{Error: "access denied", NameError: "system lookup failed", DockedNameError: "structure permission missing"}}
	got := characterESIStatus(record)
	for _, want := range []string{"Location: access denied", "System name: system lookup failed", "Station/structure name: structure permission missing"} {
		if !strings.Contains(got, want) {
			t.Fatalf("character status omitted %q: %s", want, got)
		}
	}
	for _, record := range []shipRecord{
		{Error: shipScopeMissing, Location: locationRecord{Error: locationScopeMissing, DockedNameError: structureScopeMissing}},
		{CheckedAt: now, Location: locationRecord{DockedNameError: structureScopeMissing}},
		{Scopes: []string{}},
	} {
		if got := characterESIStatus(record); got != "OK" {
			t.Fatalf("missing scopes should be OK, got %q", got)
		}
	}
	for _, record := range []shipRecord{
		{Error: "token renewal failed", Location: locationRecord{Error: locationScopeMissing}},
		{Error: shipScopeMissing, Location: locationRecord{Error: "network failure"}},
		{Error: shipScopeMissing, Location: locationRecord{DockedNameError: "structure access denied"}},
	} {
		if got := characterESIStatus(record); got == "OK" || strings.Contains(got, "tracking disabled") {
			t.Fatalf("actual failure was hidden or missing scopes shown as errors: %q", got)
		}
	}
}

func TestCharacterESIScopeLabels(t *testing.T) {
	for _, tc := range []struct {
		scopes []string
		want   string
	}{
		{nil, "Unknown"}, {[]string{}, "None"}, {[]string{shipScope}, "Ship"},
		{[]string{locationScope}, "Location"}, {[]string{structureScope}, "Structures"},
		{[]string{locationScope, shipScope}, "Ship, Location"},
		{[]string{structureScope, shipScope}, "Ship, Structures"},
		{[]string{structureScope, locationScope}, "Location, Structures"},
		{[]string{structureScope, locationScope, shipScope, "extra.scope", shipScope}, "All"},
	} {
		if got := characterESIScopes(tc.scopes); got != tc.want {
			t.Errorf("scopes %v: got %q, want %q", tc.scopes, got, tc.want)
		}
	}
}

func TestESICharacterContextMenuDisconnects(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	ui.applyShipRecords([]shipRecord{{CharacterID: 123, CharacterName: "Pilot A"}})
	ui.tabs.SelectIndex(4)
	cell := renderedSelectionCell(t, ui.esiConnections, widget.TableCellID{Row: 1})
	cell.TappedSecondary(&fyne.PointEvent{Position: fyne.NewPos(10, 10)})
	menu, ok := ui.window.Canvas().Focused().(*widget.PopUpMenu)
	if !ok {
		t.Fatal("character context menu did not open")
	}
	menu.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	menu.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	select {
	case action := <-ui.esiActions:
		if action.Kind != "disconnect" || action.CharacterID != 123 {
			t.Fatalf("wrong disconnect action: %+v", action)
		}
	default:
		t.Fatal("context menu did not disconnect the character")
	}
}
