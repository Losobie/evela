package main

import (
	"image/png"
	"os"
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func newTotalsSortTestUI(t *testing.T) (*analyzerUI, time.Time) {
	t.Helper()
	application := test.NewTempApp(t)
	window := application.NewWindow("Sortable damage totals")
	t.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"alpha", "Bravo", "charlie"}, Logs: []*parsedLog{
		{Character: "alpha", Points: []damagePoint{{Second: start.Unix(), Outgoing: 2.004, Incoming: 30, OutgoingHits: 2, IncomingHits: 9}}},
		{Character: "Bravo", Points: []damagePoint{{Second: start.Unix(), Outgoing: 100, Incoming: 2, OutgoingHits: 2, IncomingHits: 10}}},
		{Character: "charlie", Points: []damagePoint{{Second: start.Unix(), Outgoing: 2.003, Incoming: 10, OutgoingHits: 1, IncomingHits: 50}}},
	}}
	for _, name := range ui.snapshot.Characters {
		ui.selected[name] = true
	}
	ui.applyGraphRange(start, start.Add(10*time.Second))
	applyTestShipRecords(ui, []shipRecord{
		{CharacterName: "alpha", TypeID: 1, TypeName: "Rifter", ClassName: "Frigate", CheckedAt: time.Now()},
		{CharacterName: "Bravo", TypeID: 2, TypeName: "Caracal", ClassName: "Cruiser", CheckedAt: time.Now()},
		{CharacterName: "charlie", TypeID: 1, TypeName: "Rifter", ClassName: "Frigate", CheckedAt: time.Now()},
	})
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	ui.tabs.SelectIndex(1)
	return ui, start
}

func totalsNames(ui *analyzerUI) []string {
	var names []string
	for _, series := range ui.tableSeries {
		names = append(names, series.Name)
	}
	return names
}

func TestDamageTotalsColumnSortingAndRepeatedHeaderClicks(t *testing.T) {
	for _, item := range []struct {
		name            string
		column          int
		firstDescending bool
		first, next     []string
	}{
		{"name", 0, true, []string{"charlie", "Bravo", "alpha"}, []string{"alpha", "Bravo", "charlie"}},
		{"ship", 1, false, []string{"Bravo", "alpha", "charlie"}, []string{"alpha", "charlie", "Bravo"}},
		{"outgoing", 2, true, []string{"Bravo", "alpha", "charlie"}, []string{"charlie", "alpha", "Bravo"}},
		{"incoming", 3, true, []string{"alpha", "charlie", "Bravo"}, []string{"Bravo", "charlie", "alpha"}},
		{"average outgoing", 4, true, []string{"Bravo", "alpha", "charlie"}, []string{"charlie", "alpha", "Bravo"}},
		{"average incoming", 5, true, []string{"alpha", "charlie", "Bravo"}, []string{"Bravo", "charlie", "alpha"}},
		{"hits outgoing then incoming", 6, true, []string{"Bravo", "alpha", "charlie"}, []string{"charlie", "alpha", "Bravo"}},
	} {
		t.Run(item.name, func(t *testing.T) {
			ui, _ := newTotalsSortTestUI(t)
			graphs := append([]plotSeries(nil), ui.outgoing.data.Series...)
			historyLength := len(ui.graphUndo)
			ui.totals.Select(widget.TableCellID{Row: 0, Col: item.column})
			if got := totalsNames(ui); !reflect.DeepEqual(got, item.first) {
				t.Fatalf("first heading click gave %v, want %v", got, item.first)
			}
			header := newTotalsCell()
			ui.totals.UpdateCell(widget.TableCellID{Row: 0, Col: item.column}, header)
			wantIcon := theme.MoveUpIcon()
			if item.firstDescending {
				wantIcon = theme.MoveDownIcon()
			}
			if !header.icon.Visible() || header.icon.Resource.Name() != wantIcon.Name() {
				t.Fatal("heading has the wrong sort indicator")
			}
			ui.totals.Select(widget.TableCellID{Row: 0, Col: item.column})
			if got := totalsNames(ui); !reflect.DeepEqual(got, item.next) {
				t.Fatalf("repeated click did not reverse the order: %v, want %v", got, item.next)
			}
			ui.totals.UpdateCell(widget.TableCellID{Row: 0, Col: item.column}, header)
			if !header.icon.Visible() || header.icon.Resource.Name() == wantIcon.Name() {
				t.Fatal("repeated click did not reverse the heading arrow")
			}
			ui.totals.Select(widget.TableCellID{Row: 1, Col: item.column})
			if !reflect.DeepEqual(totalsNames(ui), item.next) || !reflect.DeepEqual(ui.outgoing.data.Series, graphs) || len(ui.graphUndo) != historyLength {
				t.Fatal("body selection or sorting changed graph data, row order or range history")
			}
		})
	}
}

func TestDamageTotalsSortSurvivesRefreshAndGrouping(t *testing.T) {
	ui, start := newTotalsSortTestUI(t)
	test.TapAt(ui.totals, fyne.NewPos(20, 20))
	if ui.tableSeries[0].Name != "charlie" {
		t.Fatal("tapping the visible heading did not sort the table")
	}
	test.TapAt(ui.totals, fyne.NewPos(20, 20))
	if ui.tableSeries[0].Name != "alpha" {
		t.Fatal("tapping the same visible heading did not reverse sorting")
	}
	ui.totals.Select(widget.TableCellID{Row: 0, Col: 2})
	if path := os.Getenv("EVELA_TOTALS_CAPTURE"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, ui.window.Canvas().Capture())
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	ui.snapshot.Logs[2].Points = append(ui.snapshot.Logs[2].Points, damagePoint{Second: start.Unix() + 1, Outgoing: 200, OutgoingHits: 1})
	ui.updateRange()
	if got := totalsNames(ui); !reflect.DeepEqual(got, []string{"charlie", "Bravo", "alpha"}) || ui.totalsSortColumn != 2 || !ui.totalsSortDescending {
		t.Fatalf("refresh lost the sort or used old damage values: %v", got)
	}
	if ui.view.Series[0].Name != "alpha" || ui.outgoing.data.Series[0].Name != "alpha" {
		t.Fatal("sorting table rows reordered the view or graph series")
	}
	ui.selected["charlie"] = false
	ui.renderSelection()
	if ui.tableSeries[0].Name != "Bravo" || len(ui.tableSeries) != 2 || len(ui.outgoing.data.Series) != 2 {
		t.Fatal("character filtering did not update totals while preserving their sort")
	}
	ui.selected["charlie"] = true
	ui.renderSelection()
	ui.groupSelect.SetSelected("Ship history")
	if !reflect.DeepEqual(totalsNames(ui), []string{"Rifter", "Caracal"}) {
		t.Fatal("ship-type grouping did not preserve damage sorting")
	}
	ui.totals.Select(widget.TableCellID{Row: 0, Col: 1})
	if ui.tableSeries[0].Name != "Rifter" || !ui.totalsSortDescending {
		t.Fatal("Members did not sort by numeric member count, highest first")
	}
	ui.totals.Select(widget.TableCellID{Row: 0, Col: 1})
	if ui.tableSeries[0].Name != "Caracal" || ui.totalsSortDescending {
		t.Fatal("Members could not be sorted ascending")
	}
	ui.groupSelect.SetSelected("Ship class history")
	ui.updateRange()
	if !reflect.DeepEqual(totalsNames(ui), []string{"Cruiser", "Frigate"}) || ui.totalsSortColumn != 1 || ui.totalsSortDescending || ui.totals.StickyRowCount != 1 {
		t.Fatal("class grouping/refresh lost member sorting or the fixed heading")
	}
	// Empty ranges keep the selected sort so returning to populated data restores it.
	ui.applyGraphRange(start.Add(time.Hour), start.Add(2*time.Hour))
	if len(ui.tableSeries) != 0 {
		t.Fatal("empty range retained old table rows")
	}
	ui.undoGraphRange()
	if !reflect.DeepEqual(totalsNames(ui), []string{"Cruiser", "Frigate"}) || ui.totalsSortColumn != 1 {
		t.Fatal("range undo lost the table sort")
	}
}
