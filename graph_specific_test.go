package main

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func groupByButtons(root fyne.CanvasObject) []*widget.Button {
	var buttons []*widget.Button
	switch object := root.(type) {
	case *widget.Button:
		if object.Text == "Group By" {
			buttons = append(buttons, object)
		}
	case *fyne.Container:
		for _, child := range object.Objects {
			buttons = append(buttons, groupByButtons(child)...)
		}
	case *container.Scroll:
		buttons = append(buttons, groupByButtons(object.Content)...)
	}
	return buttons
}

func TestGroupByControlsBelongToTheirGraphCards(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	for _, option := range graphVisibilityOptions {
		buttons := groupByButtons(ui.graphCards[option.id].Content)
		if len(buttons) != 1 {
			t.Fatalf("%s has %d Group By buttons, want one", option.label, len(buttons))
		}
		ui.graphsScroll.ScrollToOffset(fyne.NewPos(0, ui.graphCards[option.id].Position().Y))
		test.Tap(buttons[0])
		popup := ui.graphMenuPopup
		bounds := ui.window.Canvas().Size()
		if popup == nil || !popup.Visible() || popup.Position().Y < 0 || popup.Position().Y+popup.Size().Height > bounds.Height {
			t.Fatalf("%s Group By menu is outside the window", option.label)
		}
		popup.Hide()
	}
}

func TestEventGraphsHaveIndependentGroupingAndSavedChoices(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	ui.setAllLogisticsTypes(graphOutgoingLogistics, true)
	ui.setAllLogisticsTypes(graphIncomingLogistics, true)
	ui.shipHistory = shipHistory{1: {{CharacterName: "Logi", TypeID: 1, TypeName: "Guardian", GroupID: 832, ClassName: "Logistics", FirstSeen: start.Add(-time.Second), LastSeen: start.Add(8 * time.Second)}}}
	ui.characterGroups = []characterGroup{{ID: "logi", Name: "Logistics fleet", Members: []string{"Logi"}}}
	ui.rebuildView()
	in, ew, outDamage, totals := ui.incomingLogistics.data, ui.ewar.data, ui.outgoing.data, append([]characterSeries(nil), ui.tableSeries...)
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphOutgoingLogistics), eventByCharacter)
	if len(ui.outgoingLogistics.data.Series) != 3 || findLogisticsSeries(t, ui.outgoingLogistics, "Logi · GJ").Hits[0].Damage != 311 {
		t.Fatal("logistics Character grouping lost units or data")
	}
	if !reflect.DeepEqual(ui.incomingLogistics.data, in) || !reflect.DeepEqual(ui.ewar.data, ew) || !reflect.DeepEqual(ui.outgoing.data, outDamage) || !reflect.DeepEqual(ui.tableSeries, totals) {
		t.Fatal("logistics grouping affected another graph or Totals")
	}
	out := ui.outgoingLogistics.data
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphIncomingLogistics), eventByShip)
	if findLogisticsSeries(t, ui.incomingLogistics, "Guardian · GJ").Hits[0].Damage != 291 || !reflect.DeepEqual(ui.outgoingLogistics.data, out) {
		t.Fatal("incoming logistics grouping was not independent")
	}
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphIncomingEWar), eventByGroup)
	if len(ui.ewar.data.Series) != 1 || ui.ewar.data.Series[0].Name != "Logistics fleet" {
		t.Fatal("E-War character grouping failed")
	}
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphOutgoingLogistics), eventByClass)
	if findLogisticsSeries(t, ui.outgoingLogistics, "Logistics · HP").Hits[0].Damage != 50 {
		t.Fatal("event-time ship class grouping failed")
	}
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByAttacker)
	before := ui.outgoingLogistics.data
	chooseGraphMenuItem(t, ui.graphFormatMenu(), "Character Group")
	if !reflect.DeepEqual(ui.outgoingLogistics.data, before) || ui.eventGraphGroupings[graphIncomingLogistics] != eventByShip || ui.eventGraphGroupings[graphIncomingEWar] != eventByGroup || ui.incomingGrouping != incomingByAttacker {
		t.Fatal("outgoing grouping leaked into other graphs")
	}
	other := fyne.CurrentApp().NewWindow("Restored graph groupings")
	defer other.Close()
	restored := newAnalyzerUI(other)
	if restored.eventGraphGroupings[graphOutgoingLogistics] != eventByClass || restored.eventGraphGroupings[graphIncomingLogistics] != eventByShip || restored.eventGraphGroupings[graphIncomingEWar] != eventByGroup {
		t.Fatal("per-graph groupings did not persist")
	}
}

func TestLogisticsTypeCheckboxesAllSearchNoneAndPersistence(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	ui.setAllLogisticsTypes(graphOutgoingLogistics, true)
	beforeIncoming, totals := ui.incomingLogistics.data, append([]characterSeries(nil), ui.tableSeries...)
	ui.graphsScroll.ScrollToOffset(fyne.NewPos(0, ui.graphCards[graphOutgoingLogistics].Position().Y))
	test.Tap(ui.logisticsTypeButtons[graphOutgoingLogistics])
	popup := ui.filterPopup
	if popup.kind != "Type" || !popup.all.Checked || len(popup.visible) != 3 || popup.visible[0].Label != "Shield" || popup.visible[1].Label != "Armor" || popup.visible[2].Label != "Capacitor" {
		t.Fatal("Type popup does not expose All, Shield, Armor and Capacitor")
	}
	ui.setLogisticsType(graphOutgoingLogistics, "Armor repair", false)
	if !popup.all.Partial || popup.all.Checked || ui.logisticsTypeButtons[graphOutgoingLogistics].Text != "Type: 2/3" || len(ui.outgoingLogistics.data.Series) != 2 {
		t.Fatal("independent type checkbox or partial All state failed")
	}
	popup.search.SetText("shield")
	if len(popup.visible) != 1 {
		t.Fatal("Type search failed")
	}
	test.Tap(popup.all)
	if len(ui.outgoingLogistics.data.Series) != 3 || !popup.all.Checked {
		t.Fatal("All did not include search-hidden types")
	}
	test.Tap(popup.all)
	if len(ui.outgoingLogistics.data.Series) != 0 || ui.logisticsTypeButtons[graphOutgoingLogistics].Text != "Type: None" {
		t.Fatal("All did not clear all types")
	}
	ui.setLogisticsType(graphOutgoingLogistics, "Capacitor transfer", true)
	if ui.outgoingLogistics.data.SecondaryUnit != "" || ui.outgoingLogistics.data.HitUnit != "GJ" || len(ui.outgoingLogistics.data.Series) != 1 {
		t.Fatal("capacitor-only filter retained repair data or wrong units")
	}
	if !reflect.DeepEqual(ui.incomingLogistics.data, beforeIncoming) || !reflect.DeepEqual(ui.tableSeries, totals) {
		t.Fatal("outgoing Type filter changed another graph or Totals")
	}
	popup.search.SetText("")
	if path := os.Getenv("EVELA_LOGISTICS_TYPES_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path)
	}
	popup.popup.Hide()
	other := fyne.CurrentApp().NewWindow("Restored Type filter")
	defer other.Close()
	restored := newAnalyzerUI(other)
	if !restored.logisticsTypes[graphOutgoingLogistics].selected("Capacitor transfer") || restored.logisticsTypes[graphOutgoingLogistics].selected("Shield repair") || restored.logisticsTypes[graphOutgoingLogistics].selected("Armor repair") || !restored.logisticsTypes[graphIncomingLogistics].selected("Shield repair") {
		t.Fatal("independent Type filters did not persist")
	}
}

func TestMixedLogisticsAxesAndHoverKeepHPAndGJSeparate(t *testing.T) {
	ui, start := newLogisticsTestUI(t)
	ui.setAllLogisticsTypes(graphOutgoingLogistics, true)
	data := ui.outgoingLogistics.data
	if data.SecondaryUnit != "GJ" || data.Maximum != 54 || math.Abs(data.SecondaryMaximum-311*1.08) > 1e-10 || data.HitUnit != "HP" {
		t.Fatalf("mixed logistics axes: %+v", data)
	}
	armor := findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair")
	cap := findLogisticsSeries(t, ui.outgoingLogistics, "Capacitor transfer")
	if math.Abs(data.seriesFraction(armor, 50)-data.seriesFraction(cap, 311)) > 1e-10 {
		t.Fatal("HP and GJ did not use independent scales")
	}
	width := ui.outgoingLogistics.Size().Width - chartPlotLeft - ui.outgoingLogistics.plotRight()
	ui.outgoingLogistics.MouseMoved(testMouseEvent(chartPlotLeft+width*.25, 60, 0))
	if text := ui.outgoingLogistics.hoverReadings[0].Text; !strings.Contains(text, "311 GJ") || strings.Contains(text, "311 HP") {
		t.Fatalf("mixed hover units: %q", text)
	}
	ui.graphAverageWindow = dpsAverageWindow
	ui.renderSelection()
	if got := findLogisticsSeries(t, ui.outgoingLogistics, "Capacitor transfer").Values[2]; math.Abs(got-31.1) > 1e-10 {
		t.Fatal("Type All changed capacitor averages")
	}
	ui.outgoingLogistics.MouseMoved(testMouseEvent(chartPlotLeft+width*.25, 60, 0))
	var readings string
	for _, reading := range ui.outgoingLogistics.hoverReadings {
		readings += reading.Text
	}
	if !strings.Contains(readings, "GJ/s") || !strings.Contains(readings, "HP/s") {
		t.Fatalf("mixed averaged hover omitted per-series units: %s", readings)
	}
	// Mixed axes reserve a right margin; the pointer must still zoom correctly.
	ui.outgoingLogistics.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(chartPlotLeft+width/2, 60)}, Scrolled: fyne.NewDelta(0, 25)})
	if !ui.view.Start.After(start) || !ui.outgoingLogistics.data.End.Equal(ui.incoming.data.End) {
		t.Fatal("mixed-axis wheel zoom used the wrong plot bounds")
	}
	ui.undoGraphRange()
	if ui.view.End.Sub(ui.view.Start) != 8*time.Second {
		t.Fatal("mixed-axis navigation broke undo")
	}
	ui.graphsScroll.ScrollToOffset(fyne.NewPos(0, ui.graphCards[graphOutgoingLogistics].Position().Y))
	if path := os.Getenv("EVELA_LOGISTICS_TYPES_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path+".mixed.png")
	}
}

func TestLogisticsTypeDefaultsAndLegacyMigration(t *testing.T) {
	for _, legacy := range []string{"", "Shield & armor reps (HP)", "Capacitor transfers (GJ)"} {
		t.Run(legacy, func(t *testing.T) {
			app := test.NewTempApp(t)
			app.Preferences().SetString("graph.logistics.outgoing.metric", legacy)
			window := app.NewWindow("Type migration")
			defer window.Close()
			ui := newAnalyzerUI(window)
			selection := ui.logisticsTypes[graphOutgoingLogistics]
			if selection.selected("Armor repair") != (legacy != "Capacitor transfers (GJ)") || selection.selected("Capacitor transfer") != (legacy != "Shield & armor reps (HP)") {
				t.Fatal("Type defaults or migration reset a saved choice")
			}
		})
	}
}
