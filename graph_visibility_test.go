package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

func assertLiveVisibilityCheckmark(t *testing.T, row fyne.CanvasObject, want bool) {
	t.Helper()
	var text *canvas.Text
	var check *canvas.Image
	for _, object := range test.WidgetRenderer(row.(fyne.Widget)).Objects() {
		switch object := object.(type) {
		case *canvas.Text:
			text = object
		case *canvas.Image:
			check = object
		}
	}
	if check == nil || check.Visible() != want {
		t.Fatal("open Options menu has a stale checkmark")
	}
	if want && (text == nil || text.Position().X < check.Position().X+check.Size().Width) {
		t.Fatal("visibility checkmark overlaps its label")
	}
}

func TestOptionsStaysOpenForRepeatedVisibilityToggles(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	test.Tap(ui.graphOptionsButton)
	popup, position := ui.graphMenuPopup, ui.graphMenuPopup.Position()
	for index, option := range graphVisibilityOptions {
		row := popup.Items[index+3]
		assertLiveVisibilityCheckmark(t, row, true)
		test.Tap(row.(fyne.Tappable))
		assertGraphVisibility(t, ui, option.id, false)
		assertLiveVisibilityCheckmark(t, row, false)
		if ui.graphMenuPopup != popup || !popup.Visible() || popup.Position() != position || len(ui.window.Canvas().Overlays().List()) != 1 {
			t.Fatal("toggling visibility closed, moved or duplicated Options")
		}
	}
	if ui.graphsScroll.Content != ui.graphsEmpty {
		t.Fatal("could not hide all graphs without closing Options")
	}
	// Keyboard activation stays on the toggled row and can toggle it repeatedly.
	for _, want := range []bool{true, false, true} {
		popup.TypedKey(&fyne.KeyEvent{Name: fyne.KeySpace})
		assertGraphVisibility(t, ui, graphIncomingEWar, want)
		assertLiveVisibilityCheckmark(t, popup.Items[7], want)
		if !popup.Visible() {
			t.Fatal("keyboard visibility toggle closed Options")
		}
	}
	popup.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if popup.Visible() || ui.window.Canvas().Overlays().Top() != nil {
		t.Fatal("Escape did not dismiss Options")
	}
	test.Tap(ui.graphOptionsButton)
	popup = ui.graphMenuPopup
	test.TapCanvas(ui.window.Canvas(), fyne.NewPos(10, 10))
	if popup.Visible() || ui.window.Canvas().Overlays().Top() != nil {
		t.Fatal("outside click did not dismiss Options")
	}
	// Standard grouping menus still dismiss after a choice.
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show Outgoing")
	ui.graphsScroll.ScrollToTop()
	test.Tap(ui.graphFormatButton)
	grouping := ui.graphMenuPopup
	test.Tap(grouping.Items[0].(fyne.Tappable))
	if grouping.Visible() {
		t.Fatal("Group By inherited the sticky Options behavior")
	}
}

func TestOptionsCanShowGraphsWhenOpenedWithEveryToggleOff(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	for _, option := range graphVisibilityOptions {
		chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show "+option.label)
	}
	test.Tap(ui.graphOptionsButton)
	popup := ui.graphMenuPopup
	test.Tap(popup.Items[3].(fyne.Tappable))
	assertGraphVisibility(t, ui, graphOutgoing, true)
	assertLiveVisibilityCheckmark(t, popup.Items[3], true)
	if !popup.Visible() {
		t.Fatal("Options closed when restoring the first graph")
	}
}

func assertGraphVisibility(t *testing.T, ui *analyzerUI, id string, want bool) {
	t.Helper()
	if ui.graphCards[id].Visible() != want {
		t.Fatalf("graph %s visible=%v, want %v", id, ui.graphCards[id].Visible(), want)
	}
	for _, option := range graphVisibilityOptions {
		if option.id != id {
			continue
		}
		for _, item := range ui.graphOptionsMenu().Items {
			if item.Label == "Show "+option.label {
				if item.Checked != want {
					t.Fatalf("menu %s checked=%v, want %v", item.Label, item.Checked, want)
				}
				return
			}
		}
	}
	t.Fatalf("missing visibility menu for %s", id)
}

func TestGraphVisibilityCollapsesCardsWithoutChangingAnalysis(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	data, totals, summary := ui.outgoingLogistics.data, append([]characterSeries(nil), ui.tableSeries...), ui.summary.Text
	beforeRange, history := graphHistoryState(ui), len(ui.graphUndo)
	for _, option := range graphVisibilityOptions {
		assertGraphVisibility(t, ui, option.id, true)
		beforeHeight := ui.graphStack.MinSize().Height
		chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show "+option.label)
		assertGraphVisibility(t, ui, option.id, false)
		if ui.graphStack.MinSize().Height >= beforeHeight {
			t.Fatal("hidden graph retained a blank space")
		}
		for _, other := range graphVisibilityOptions {
			if other.id != option.id {
				assertGraphVisibility(t, ui, other.id, true)
			}
		}
		if !reflect.DeepEqual(ui.outgoingLogistics.data, data) || !reflect.DeepEqual(ui.tableSeries, totals) || ui.summary.Text != summary || graphHistoryState(ui) != beforeRange || len(ui.graphUndo) != history {
			t.Fatal("visibility changed data, totals, summary or range history")
		}
		chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show "+option.label)
		assertGraphVisibility(t, ui, option.id, true)
		if ui.graphStack.MinSize().Height != beforeHeight {
			t.Fatal("restoring graph did not restore its layout space")
		}
	}
	// A hidden graph must still pick up fresh logs and shared options, so showing
	// it later cannot reveal stale series or a stale time axis.
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show Outgoing logistics")
	ui.snapshot.Logs[0].Logistics = append(ui.snapshot.Logs[0].Logistics, logisticsEvent{Second: ui.view.Start.Unix(), Kind: "Armor repair", Amount: 200})
	ui.rebuildView()
	if hits := findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Hits; len(hits) != 3 {
		t.Fatal("hidden graph did not refresh its data")
	}
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "10-second DPS average")
	ui.applyGraphRange(ui.view.Start, ui.view.Start.Add(4*time.Second))
	assertGraphVisibility(t, ui, graphOutgoingLogistics, false)
	chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show Outgoing logistics")
	if ui.outgoingLogistics.data.AverageWindow != dpsAverageWindow || findLogisticsSeries(t, ui.outgoingLogistics, "Armor repair").Values[0] != 35 || !ui.outgoingLogistics.data.End.Equal(ui.view.End) {
		t.Fatal("restored graph did not use current data, options and range")
	}
}

func TestGraphVisibilityAllHiddenPersistsAndCanBeRestored(t *testing.T) {
	ui, _ := newLogisticsTestUI(t)
	ui.graphsScroll.ScrollToBottom()
	for _, option := range graphVisibilityOptions {
		chooseGraphMenuItem(t, ui.graphOptionsMenu(), "Show "+option.label)
	}
	if ui.graphsScroll.Content != ui.graphsEmpty || ui.graphsScroll.Offset.Y != 0 {
		t.Fatal("all-hidden state did not replace the graphs and clear obsolete scroll offset")
	}
	// Options remains outside the scroller and usable with every graph hidden.
	test.Tap(ui.graphOptionsButton)
	popup := ui.graphMenuPopup
	bounds := ui.window.Canvas().Size()
	if popup == nil || !popup.Visible() || popup.Position().Y+popup.Size().Height > bounds.Height || popup.Position().X+popup.Size().Width > bounds.Width {
		t.Fatal("visibility Options dropdown is inaccessible or outside the window")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_GRAPH_VISIBILITY_CAPTURE"))
	popup.Hide()
	window := fyne.CurrentApp().NewWindow("Restored hidden graphs")
	defer window.Close()
	restored := newAnalyzerUI(window)
	window.SetContent(restored.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	for _, option := range graphVisibilityOptions {
		assertGraphVisibility(t, restored, option.id, false)
	}
	if restored.graphsScroll.Content != restored.graphsEmpty {
		t.Fatal("saved all-hidden choice was not applied before loading logs")
	}
	chooseGraphMenuItem(t, restored.graphOptionsMenu(), "Show Incoming")
	assertGraphVisibility(t, restored, graphIncoming, true)
	if restored.graphsScroll.Content != restored.graphStack {
		t.Fatal("showing a graph did not dismiss the empty state")
	}
	for _, option := range graphVisibilityOptions {
		if option.id != graphIncoming {
			assertGraphVisibility(t, restored, option.id, false)
		}
	}
	if restored.graphCards[graphIncoming].Position().Y != 0 {
		t.Fatal("hidden cards left gaps above the visible graph")
	}
	ui.applyGraphVisibility()
	assertGraphVisibility(t, ui, graphIncoming, true)
}
