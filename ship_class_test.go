package main

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestESIShipClassesCachePersistAndRetry(t *testing.T) {
	f := newESIFixture(t)
	account := &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"}, Token: esiToken{AccessToken: f.token, ExpiresAt: time.Now().Add(time.Hour)}}
	f.service.config.Accounts[123] = account
	f.groupStatus.Store(http.StatusServiceUnavailable)
	if err := f.service.refreshShips(context.Background()); err == nil || !strings.Contains(err.Error(), "ship class") {
		t.Fatalf("class lookup failure was not reported: %v", err)
	}
	if !account.Ship.current() || account.Ship.TypeName != "Loki" || account.Ship.ClassName != "" || account.Ship.ClassError == "" {
		t.Fatalf("class failure discarded the confirmed hull: %+v", account.Ship)
	}
	f.groupStatus.Store(0)
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if account.Ship.GroupID != 963 || account.Ship.ClassName != "Strategic Cruiser" || account.Ship.ClassError != "" || f.typeRequests.Load() != 1 || f.groupRequests.Load() != 2 {
		t.Fatalf("class lookup did not recover using the cached hull: %+v", account.Ship)
	}
	// A different hull in the same class reuses the public group lookup.
	f.shipType.Store(29984)
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if account.Ship.TypeName != "Tengu" || account.Ship.ClassName != "Strategic Cruiser" || f.typeRequests.Load() != 2 || f.groupRequests.Load() != 2 {
		t.Fatal("ship change lost the class or fetched the same group again")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if ship := loaded.config.Accounts[123].Ship; ship.ClassName != "Strategic Cruiser" || ship.GroupID != 963 || ship.current() {
		t.Fatalf("saved class was lost or treated as a fresh ship on restart: %+v", ship)
	}
}

func TestShipClassFormatSynchronizesGraphsLegendAndTotals(t *testing.T) {
	application := test.NewTempApp(t)
	application.Preferences().SetBool(graphAveragePreference, false)
	window := application.NewWindow("Ship class format")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	ui.snapshot = &logSnapshot{Characters: []string{"A", "B"}, Logs: []*parsedLog{
		{Character: "A", Points: []damagePoint{{Second: start.Unix(), Outgoing: 100, Incoming: 10, OutgoingHits: 1}}},
		{Character: "B", Points: []damagePoint{{Second: start.Unix(), Outgoing: 200, Incoming: 20, OutgoingHits: 1}}},
	}}
	ui.selected["A"], ui.selected["B"] = true, true
	ui.bucketSelect.SetSelected("1 second")
	ui.applyGraphRange(start, start.Add(20*time.Second))
	records := []shipRecord{
		{CharacterName: "A", TypeID: 1, TypeName: "Nyx", ClassName: "Supercarrier", CheckedAt: time.Now()},
		{CharacterName: "B", TypeID: 2, TypeName: "Hel", ClassName: "Supercarrier", CheckedAt: time.Now()},
	}
	applyTestShipRecords(ui, records)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	for _, item := range ui.graphFormatMenu().Items {
		if item.Label == "Ship class (ship history)" {
			item.Action()
		}
	}
	if !ui.shipClassGrouping() || len(ui.outgoing.data.Series) != 1 || ui.outgoing.data.Series[0].Name != "Supercarrier" || ui.outgoing.data.Series[0].Values[0] != 300 || len(ui.incoming.data.Series) != 2 || ui.incoming.data.Series[0].Values[0] != 10 {
		t.Fatal("outgoing class format changed incoming grouping or failed to combine outgoing damage")
	}
	if len(ui.tableSeries) != 1 || ui.tableSeries[0].TotalOutgoing != 300 || len(ui.tableSeries[0].Members) != 2 || !strings.Contains(ui.graphFormatLabel.Text, "By ship class") {
		t.Fatal("class format did not update the totals or active format label")
	}
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+1, 80, 0))
	if label := ui.outgoing.hoverReadings[0]; !strings.Contains(label.Text, "Supercarrier · B: 200 damage") || label.Color != characterColor("Supercarrier") {
		t.Fatalf("ship-class hit hover lost its character or color: %s", label.Text)
	}
	ui.incoming.MouseMoved(testMouseEvent(chartPlotLeft+1, 80, 0))
	if label := ui.incoming.hoverReadings[0]; !strings.Contains(label.Text, "B: 20 damage") || label.Color != characterColor("B") {
		t.Fatalf("incoming hover lost its independent character grouping: %s", label.Text)
	}
	for _, item := range ui.graphFormatMenu().Items {
		if (item.Label == "Ship class (ship history)" && !item.Checked) || ((item.Label == "Ship type (ship history)" || item.Label == "Character") && item.Checked) {
			t.Fatal("grouping format checkmarks disagree")
		}
	}
	header := newTotalsCell()
	ui.totals.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, header)
	if header.label.Text != "Ship class" || !header.icon.Visible() {
		t.Fatal("totals retained the ship-type header")
	}
	ui.characterSearch.SetText("supercarrier")
	if len(characterFilterChecks(ui.characters)) != 2 {
		t.Fatal("legend could not find members by class")
	}
	ui.characterSearch.SetText("hel")
	if got := characterFilterChecks(ui.characters); !reflect.DeepEqual(got, map[string]bool{"B": true}) {
		t.Fatalf("class legend lost hull search: %v", got)
	}
	ui.characterSearch.SetText("")
	ui.selected["B"] = false
	ui.graphAverage10s = true
	ui.updateRange()
	applyTestShipRecords(ui, records)
	if !ui.shipClassGrouping() || ui.outgoing.data.Series[0].Values[0] != 10 || ui.incoming.data.Series[0].Values[0] != 1 || ui.tableSeries[0].TotalOutgoing != 100 {
		t.Fatal("refresh lost class grouping, averaging or character selection")
	}
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+1, 80, 0))
	if !strings.Contains(ui.outgoing.hoverReadings[0].Text, "Supercarrier: 10.0 DPS") || ui.outgoing.hoverReadings[0].Color != characterColor("Supercarrier") {
		t.Fatal("hover did not use the class name and graph color")
	}
	ui.groupSelect.SetSelected("Ship history")
	if ui.shipClassGrouping() || ui.outgoing.data.Series[0].Name != "Nyx" {
		t.Fatal("switching back to ship types failed")
	}
}
