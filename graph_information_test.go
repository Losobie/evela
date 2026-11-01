package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func TestGraphInformationTracksOnlyApplicableOptions(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	for scope, kinds := range ui.visibleFilters {
		for kind := range kinds {
			ui.visibleFilters[scope][kind] = false
		}
	}
	ui.refreshScopedFilterBars()
	ui.rebuildView()
	for _, graph := range graphVisibilityOptions {
		label := ui.graphInformationLabels[graph.id]
		if label == nil || label.Text == "" || label.Wrapping != fyne.TextWrapWord {
			t.Fatalf("%s lacks a consistent information label", graph.id)
		}
		if strings.Contains(strings.ToLower(label.Text), "linear") || !strings.Contains(label.Text, "By ") || !strings.Contains(label.Text, "Filters: ") {
			t.Fatalf("incorrect information: %s", label.Text)
		}
		placed := false
		for _, object := range ui.graphCards[graph.id].Content.(*fyne.Container).Objects {
			if controls, ok := object.(*fyne.Container); ok && len(controls.Objects) > 1 && controls.Objects[0] == label {
				placed = true
			}
		}
		if !placed {
			t.Fatalf("%s label is not above its filters", graph.id)
		}
	}
	ui.graphAverageWindow = 30 * time.Second
	ui.renderSelection()
	for id, label := range ui.graphInformationLabels {
		if !strings.Contains(label.Text, "30-second average") || !strings.Contains(label.Text, "per second") {
			t.Fatalf("%s average missing: %s", id, label.Text)
		}
	}
	chooseGraphMenuItem(t, ui.graphFormatMenu(), filterWeaponType)
	chooseGraphMenuItem(t, ui.incomingGroupingMenu(), incomingByNPC)
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphMining), eventByOreName)
	chooseGraphMenuItem(t, ui.eventGroupingMenu(graphOutgoingLogistics), eventByClass)
	ui.ewarMetric.SetSelected(ewarCapacitor)
	for id, want := range map[string]string{graphOutgoing: "By weapon type", graphIncoming: "By npc", graphMining: "By ore name", graphOutgoingLogistics: "By ship class", graphIncomingEWar: "capacitor drained (GJ)"} {
		if !strings.Contains(ui.graphInformationLabels[id].Text, want) {
			t.Fatalf("%s missing %s: %s", id, want, ui.graphInformationLabels[id].Text)
		}
	}
	ui.setFilterVisible(globalFilterScope, filterSystem, true)
	ui.setAllScopeFilters(globalFilterScope, filterSystem, false)
	for id, label := range ui.graphInformationLabels {
		if !strings.Contains(label.Text, "System: None (global)") {
			t.Fatalf("%s global filter missing", id)
		}
	}
	ui.setFilterVisible(globalFilterScope, filterSystem, false)
	for id, label := range ui.graphInformationLabels {
		if strings.Contains(label.Text, "System:") {
			t.Fatalf("%s hidden global filter remains", id)
		}
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, true)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	if !strings.Contains(ui.graphInformationLabels[graphOutgoing].Text, "Weapon Type: None") {
		t.Fatal("local weapon filter missing")
	}
	for _, id := range []string{graphIncoming, graphMining, graphIncomingEWar, graphOutgoingLogistics, graphIncomingLogistics} {
		if strings.Contains(ui.graphInformationLabels[id].Text, "Weapon Type:") {
			t.Fatalf("local filter leaked into %s information", id)
		}
	}
	ui.setFilterVisible(graphOutgoing, filterWeaponType, false)
	ui.graphInvulnerabilityMarkers = true
	ui.graphAverageWindow = 0
	ui.renderSelection()
	for id, label := range ui.graphInformationLabels {
		want := "individual events"
		if id == graphIncoming || id == graphOutgoing {
			want = "individual hits"
		}
		if !strings.Contains(label.Text, want) || strings.Contains(label.Text, "30-second average") {
			t.Fatalf("%s individual mode missing", id)
		}
		if strings.Contains(label.Text, "Invulnerability indicators") != (id == graphOutgoing) {
			t.Fatalf("irrelevant indicator option for %s", id)
		}
	}
	if !strings.Contains(ui.graphInformationLabels[graphOutgoing].Text, "Invulnerability indicators: on") {
		t.Fatal("indicator state did not update")
	}
	// Selecting capacitor alone changes only that logistics graph's units.
	ui.setFilterVisible(graphOutgoingLogistics, filterLogisticsType, true)
	ui.setAllScopeFilters(graphOutgoingLogistics, filterLogisticsType, false)
	ui.setScopeFilter(graphOutgoingLogistics, filterLogisticsType, "Capacitor transfer", true)
	if !strings.Contains(ui.graphInformationLabels[graphOutgoingLogistics].Text, "GJ") || strings.Contains(ui.graphInformationLabels[graphOutgoingLogistics].Text, "HP") {
		t.Fatal("logistics metric did not update")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_GRAPH_INFORMATION_CAPTURE"))
}

func TestInformationFilterValuesShowSelectedNames(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	ui.setAllDamageFilterOptions(filterShipType, false)
	ui.setDamageFilterOption(filterShipType, "type:1", true)
	if !strings.Contains(ui.graphInformationLabels[graphIncoming].Text, "Ship Type: Loki (global)") {
		t.Fatal("selected filter name missing")
	}
	// The per-graph labels share the same presentation configuration.
	for _, label := range ui.graphInformationLabels {
		if label.Truncation != fyne.TextTruncateOff {
			t.Fatal("information label truncates options")
		}
	}
}
