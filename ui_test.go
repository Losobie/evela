package main

import (
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func captureWindowPreview(t *testing.T, window fyne.Window, path string) {
	t.Helper()
	if path == "" {
		return
	}
	window.Canvas().Capture() // Settle wrapped-label heights before exporting.
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

func TestUISelectionRangeAndRendering(t *testing.T) {
	application := test.NewTempApp(t)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("Evela preview")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"Araziam Semah", "Clento Loso", "Taredi Kisi Khagah"}, First: start, Last: start.Add(20 * time.Minute)}
	for c, name := range ui.snapshot.Characters {
		log := &parsedLog{Character: name}
		for second := 0; second <= 1200; second += 5 {
			out := float64((second * (c + 3)) % 2700)
			in := float64((second * (c + 1)) % 900)
			if second%190 > 100 {
				out, in = 0, 0
			}
			log.Points = append(log.Points, damagePoint{Second: start.Unix() + int64(second), Outgoing: out, Incoming: in, OutgoingHits: 1, IncomingHits: 1})
		}
		ui.snapshot.Logs = append(ui.snapshot.Logs, log)
		ui.selected[name] = true
	}
	ui.rangeSelect.SetSelected("Last hour")
	ui.rebuildCharacters()
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	if len(ui.outgoing.data.Series) != 3 || len(ui.incoming.data.Series) != 3 {
		t.Fatal("not all characters are graphed")
	}
	ui.selected["Clento Loso"] = false
	ui.renderSelection()
	if len(ui.outgoing.data.Series) != 2 || !strings.Contains(ui.summary.Text, "2 selected") {
		t.Fatal("selection did not update graphs and totals")
	}
	ui.start.SetText("2026-10-17 02:00:00")
	ui.end.SetText("2026-10-17 02:21:00")
	if ui.rangeSelect.Selected != "Custom" {
		t.Fatal("editing dates should preserve a custom range during refresh")
	}
	ui.updateRange()
	if ui.view.End.Sub(ui.view.Start) != 21*time.Minute {
		t.Fatal("custom date range not applied")
	}
	ui.selected["Clento Loso"] = true
	ui.rebuildCharacters()
	ui.renderSelection()
	ui.outgoing.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(150, 80)}})
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, "DPS") {
		t.Fatal("hover does not show damage rates")
	}
	ui.outgoing.MouseOut()
	applyTestShipRecords(ui, []shipRecord{
		{CharacterID: 1, CharacterName: "Araziam Semah", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()},
		{CharacterID: 2, CharacterName: "Clento Loso", TypeID: 29990, TypeName: "Loki", CheckedAt: time.Now()},
		{CharacterID: 3, CharacterName: "Taredi Kisi Khagah", TypeID: 29984, TypeName: "Tengu", CheckedAt: time.Now()},
	})
	ui.groupSelect.SetSelected("Ship history")
	if len(ui.outgoing.data.Series) != 2 || len(ui.tableSeries) != 2 {
		t.Fatal("ship grouping did not combine graphs and totals")
	}
	ui.selected["Clento Loso"] = false
	ui.renderSelection()
	if len(ui.outgoing.data.Series) != 2 || len(ui.tableSeries[0].Members) != 1 {
		t.Fatal("character filtering did not update graph and total memberships together")
	}
	ui.selected["Clento Loso"] = true
	ui.renderSelection()
	if path := os.Getenv("EVELA_UI_CAPTURE"); path != "" {
		ui.status.SetText("Updated 19:30:00 · Logs stay on this computer. Latest combat: 2026-10-17 02:20:00 EVE time.")
		ui.directoryInfo.SetText("32650 log files · 61 characters · 0 duplicate exports skipped · 0 files without a Listener header")
		ui.setESIStatus("Ships updated 19:30:00 · 3 connected characters.")
		for _, chart := range []*damageChart{ui.outgoing, ui.incoming} {
			width := chart.Size().Width - chartPlotLeft - chartPlotRight
			chart.MouseMoved(testMouseEvent(chartPlotLeft+width/2, 80, 0))
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		ui.tabs.SelectIndex(3)
		esiFile, err := os.Create(path + ".esi.png")
		if err != nil {
			t.Fatal(err)
		}
		defer esiFile.Close()
		if err := png.Encode(esiFile, window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
}
