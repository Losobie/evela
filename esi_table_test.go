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
		{CharacterID: 1, CharacterName: "Pilot A", TypeName: "Loki", ClassName: "Strategic Cruiser", ShipName: shipName, CheckedAt: checked, Error: "Ship lookup failed", ClassError: "Class lookup failed", Location: locationRecord{SystemName: "Jita", CheckedAt: time.Now()}},
		{CharacterID: 2, CharacterName: "Pilot B"},
	}
	ui.applyShipRecords(records)
	ui.tabs.SelectIndex(4)
	table := ui.esiConnections
	for col, heading := range []string{"Character", "Time Checked", "Ship", "Location"} {
		cell := renderedSelectionCell(t, table, widget.TableCellID{Col: col})
		label := cell.content.(*widget.Label)
		if label.Text != heading || !label.TextStyle.Bold {
			t.Fatalf("incorrect heading %d: %q", col, label.Text)
		}
	}
	if ui.esiRows[0][1] != "2026-11-16 17:34:56" || ui.esiRows[1][1] != "Not checked" {
		t.Fatalf("checked times should be EVE time without timezone text: %v", ui.esiRows)
	}
	for _, part := range []string{shipName, "Strategic Cruiser", "Ship lookup failed", "Class lookup failed"} {
		if !strings.Contains(ui.esiRows[0][2], part) {
			t.Fatalf("ship cell lost details %q", part)
		}
	}
	if ui.esiRows[0][3] != "Jita" || ui.esiRows[1][3] != "Location unavailable" {
		t.Fatalf("incorrect locations: %v", ui.esiRows)
	}
	first := renderedSelectionCell(t, table, widget.TableCellID{Row: 1, Col: 0})
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if selectionCopy(table) != "Pilot A" || ui.window.Canvas().Focused() != table {
		t.Fatal("ESI cell did not select immediately and receive clipboard focus")
	}
	last := renderedSelectionCell(t, table, widget.TableCellID{Row: 2, Col: 3})
	start := fyne.CurrentApp().Driver().AbsolutePositionForObject(first)
	end := fyne.CurrentApp().Driver().AbsolutePositionForObject(last).Add(fyne.NewPos(10, 10))
	test.MoveMouse(ui.window.Canvas(), end)
	first.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: end.Subtract(start)}})
	first.DragEnd()
	first.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	first.Tapped(&fyne.PointEvent{})
	want := strings.Join(ui.esiRows[0][:], "\t") + "\n" + strings.Join(ui.esiRows[1][:], "\t")
	if len(table.selected) != 8 || selectionCopy(table) != want {
		t.Fatalf("drag did not copy the full rectangular values: %q", selectionCopy(table))
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_ESI_TABLE_CAPTURE"))
	last.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierControl})
	if len(table.selected) != 7 {
		t.Fatal("Ctrl-click did not toggle a cell")
	}
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	last.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierShift})
	if len(table.selected) != 8 {
		t.Fatal("Shift-click did not extend the range")
	}
	ui.applyShipRecords(records[1:])
	if len(table.selected) != 0 {
		t.Fatal("refresh retained selection of replaced records")
	}
	ui.applyShipRecords(nil)
	if rows, cols := table.Length(); rows != 1 || cols != 4 {
		t.Fatalf("empty connections should show only the four headings: %dx%d", rows, cols)
	}
}

func TestESITableFitsViewport(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("ESI table layout")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	for i := 0; i < 100; i++ {
		ui.esiRows = append(ui.esiRows, [4]string{"Character", "2026-11-16 17:34:56", strings.Repeat("Long ship ", 20), strings.Repeat("Long location ", 20)})
	}
	holder := newFillingTable(ui.esiConnections, 180, 240, 280)
	window.SetContent(holder)
	window.Show()
	scroll := selectionTableScroll(t, ui.esiConnections)
	for _, width := range []float32{480, 1440, 900, 520} {
		holder.Resize(fyne.NewSize(width, 400))
		ui.esiConnections.ScrollTo(widget.TableCellID{Row: 100, Col: 3})
		contentWidth := scroll.Content.MinSize().Width
		if contentWidth > scroll.Size().Width+0.1 || contentWidth < scroll.Size().Width-0.1 || scroll.Offset.X > 0.1 || scroll.Offset.Y <= 0 {
			t.Fatalf("width %.0f: content %.1f, viewport %.1f, scroll %v", width, contentWidth, scroll.Size().Width, scroll.Offset)
		}
	}
}
