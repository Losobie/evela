package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

func TestRefreshStatusKeepsLayoutStable(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Refresh layout")
	defer window.Close()
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()

	type geometry struct {
		position fyne.Position
		size     fyne.Size
	}
	measure := func(objects ...fyne.CanvasObject) []geometry {
		window.Canvas().Capture() // Apply pending layout changes, as a rendered frame would.
		var result []geometry
		for _, object := range objects {
			result = append(result, geometry{application.Driver().AbsolutePositionForObject(object), object.Size()})
		}
		return result
	}
	assertSame := func(before []geometry, objects ...fyne.CanvasObject) {
		t.Helper()
		for index, after := range measure(objects...) {
			if after != before[index] {
				t.Fatalf("refresh moved/resized object %d: before %+v, after %+v", index, before[index], after)
			}
		}
	}
	longError := strings.Repeat("A very long refresh error including a log filename or ESI endpoint. ", 12) + "\nAdditional diagnostic details."
	for _, size := range []fyne.Size{fyne.NewSize(1440, 960), fyne.NewSize(1100, 800)} {
		window.Resize(size)
		ui.status.SetText("Ready")
		ui.directoryInfo.SetText("")
		ui.progress.Hide()
		objects := []fyne.CanvasObject{ui.outgoing, ui.incoming, ui.characterSearch, ui.refresh, ui.summary}
		before := measure(objects...)
		if before[0].position.Y <= application.Driver().AbsolutePositionForObject(ui.refresh).Y {
			t.Fatal("refresh controls should stay above the graphs")
		}
		ui.refresh.Disable()
		ui.progress.Show()
		ui.progress.SetValue(0.5)
		ui.status.SetText("Indexing 16325 / 32650 log files…")
		assertSame(before, objects...)
		ui.refresh.Enable()
		ui.progress.Hide()
		ui.status.SetText(longError)
		ui.directoryInfo.SetText("32650 log files · 61 characters · 100 duplicate exports skipped · 200 files without a Listener header")
		ui.summary.SetText(strings.Repeat("Very large damage totals ", 20))
		assertSame(before, objects...)
		if ui.status.Text != longError {
			t.Fatal("full diagnostics for the Details dialog were lost")
		}
	}

	ui.tabs.SelectIndex(3)
	ui.setESIStatus("Ships updated")
	objects := []fyne.CanvasObject{ui.esiConnections, ui.shipRefresh, ui.esiStatus}
	before := measure(objects...)
	ui.shipRefresh.Disable()
	ui.setESIStatus("Querying current ships from ESI…")
	assertSame(before, objects...)
	ui.shipRefresh.Enable()
	ui.setESIStatus(longError)
	assertSame(before, objects...)

	ui.openESISettings()
	ui.setESIStatus("Ready")
	objects = []fyne.CanvasObject{ui.esiSettingsTabs, ui.esiDialogStatus}
	before = measure(objects...)
	ui.setESIStatus(longError)
	assertSame(before, objects...)
	ui.esiDialog.Hide()
}

func TestRefreshPreservesListScrollPositions(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Refresh scroll position")
	defer window.Close()
	ui := newAnalyzerUI(window)
	var records []shipRecord
	start := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	ui.snapshot.First, ui.snapshot.Last = start, start.Add(time.Hour)
	for index := 0; index < 40; index++ {
		name := fmt.Sprintf("Character %02d", index)
		ui.snapshot.Characters = append(ui.snapshot.Characters, name)
		ui.selected[name] = true
		ui.snapshot.Logs = append(ui.snapshot.Logs, &parsedLog{Character: name, Points: []damagePoint{{Second: start.Unix(), Outgoing: 100}}})
		records = append(records, shipRecord{CharacterID: int64(index + 1), CharacterName: name, TypeID: 1, TypeName: "Loki", CheckedAt: time.Now()})
	}
	ui.rangeSelect.SetSelected("All history")
	ui.updateRange()
	applyTestShipRecords(ui, records)
	characters := container.NewVScroll(ui.characters)
	ships := container.NewVScroll(ui.esiConnections)
	tokens := container.NewVScroll(ui.tokenConnections)
	window.SetContent(container.NewGridWithColumns(3, characters, ships, tokens))
	window.Resize(fyne.NewSize(1200, 500))
	window.Show()

	for _, grouping := range []string{"Characters", "Ship history"} {
		ui.groupSelect.SetSelected(grouping)
		for _, scroll := range []*container.Scroll{characters, ships, tokens} {
			scroll.ScrollToOffset(fyne.NewPos(0, 350))
			if scroll.Offset.Y != 350 {
				t.Fatal("fixture did not provide enough scrollable content")
			}
		}
		ui.updateRange()                  // Log refresh.
		applyTestShipRecords(ui, records) // ESI refresh.
		window.Canvas().Capture()
		for _, scroll := range []*container.Scroll{characters, ships, tokens} {
			if scroll.Offset.Y != 350 {
				t.Fatalf("%s refresh reset scroll position to %v", grouping, scroll.Offset)
			}
		}
	}
}
