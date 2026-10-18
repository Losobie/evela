package main

import (
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestShipHistoryViewer(t *testing.T) {
	application := test.NewTempApp(t)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Ship history preview")
	defer window.Close()
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	ui.openShipHistory()
	if window.Canvas().Overlays().Top() == nil {
		t.Fatal("empty history dialog did not open")
	}
	window.Canvas().Overlays().Remove(window.Canvas().Overlays().Top())
	start := time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC)
	ui.shipHistory = shipHistory{
		123: {
			{CharacterID: 123, CharacterName: "Araziam Semah", ShipID: 1000000001, TypeID: 29990, TypeName: "Loki", ShipName: "First ship", FirstSeen: start, LastSeen: start.Add(10 * time.Minute), Observations: 21},
			{CharacterID: 123, CharacterName: "Araziam Semah", ShipID: 1000000002, TypeID: 29984, TypeName: "Tengu", ShipName: "Second ship", FirstSeen: start.Add(11 * time.Minute), LastSeen: start.Add(30 * time.Minute), Observations: 39},
		},
		456: {{CharacterID: 456, CharacterName: "Disconnected Pilot", TypeID: 29990, FirstSeen: start, LastSeen: start, Observations: 1}},
	}
	content := shipHistoryContent(ui.shipHistory).(*fyne.Container)
	var table *widget.Table
	for _, object := range content.Objects {
		if candidate, ok := object.(*widget.Table); ok {
			table = candidate
		}
	}
	if table == nil {
		t.Fatal("history viewer has no table")
	}
	if rows, columns := table.Length(); rows != 4 || columns != 7 {
		t.Fatalf("history table dimensions = %d x %d", rows, columns)
	}
	label := widget.NewLabel("")
	for col, want := range []string{"Araziam Semah", "Tengu", "Second ship", "1000000002", "2026-10-17 01:11:00", "2026-10-17 01:30:00", "39"} {
		table.UpdateCell(widget.TableCellID{Row: 1, Col: col}, label)
		if label.Text != want {
			t.Fatalf("column %d = %q, want %q", col, label.Text, want)
		}
	}
	table.UpdateCell(widget.TableCellID{Row: 3, Col: 1}, label)
	if label.Text != "Type 29990" {
		t.Fatal("unresolved type ID was hidden")
	}
	table.UpdateCell(widget.TableCellID{Row: 3, Col: 3}, label)
	if label.Text != "Unknown" {
		t.Fatal("legacy missing item ID was invented")
	}
	ui.shipHistory[123][1].ShipName = "Changed after opening"
	table.UpdateCell(widget.TableCellID{Row: 1, Col: 2}, label)
	if strings.Contains(label.Text, "Changed") {
		t.Fatal("refresh mutated an open history snapshot")
	}
	ui.shipHistory[123][1].ShipName = "Second ship"
	ui.openShipHistory()
	if path := os.Getenv("EVELA_HISTORY_CAPTURE"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
}
