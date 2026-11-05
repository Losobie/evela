package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func TestSeriesSelectionEmphasisAndRefresh(t *testing.T) {
	chart, window, data := newLineHoverTestChart(t)
	before := chart.data
	position := hoverLinePosition(chart, .4, data.Series[4], 20)
	click := func(at fyne.Position) {
		chart.MouseDown(testMouseEvent(at.X, at.Y, desktop.MouseButtonPrimary))
		chart.MouseUp(testMouseEvent(at.X, at.Y, desktop.MouseButtonPrimary))
	}
	click(position)
	if chart.selectedSeries == nil || *chart.selectedSeries != seriesKey(data.Series[4]) {
		t.Fatal("click did not select the lower series")
	}
	chart.MouseMoved(testMouseEvent(position.X, chartPlotTop+1, 0))
	if !strings.Contains(chart.hoverReadings[0].Text, "Line 5") || !reflect.DeepEqual(before, chart.data) {
		t.Fatal("selection did not prioritize its readout or changed graph data")
	}
	selectedImage := chart.renderPlot(1000, 250)
	chart.setData(data)
	if chart.selectedSeries == nil {
		t.Fatal("normal refresh cleared selection")
	}
	if path := os.Getenv("EVELA_SELECTION_CAPTURE"); path != "" {
		captureWindowPreview(t, window, path)
	}
	click(position)
	if chart.selectedSeries != nil || reflect.DeepEqual(selectedImage, chart.renderPlot(1000, 250)) {
		t.Fatal("second click did not clear emphasis or rendering did not change")
	}
	click(position)
	click(fyne.NewPos(position.X, chartPlotTop+1))
	if chart.selectedSeries != nil {
		t.Fatal("click on blank plot space did not clear selection")
	}
	click(position)
	data.Series = data.Series[:4]
	chart.setData(data)
	if chart.selectedSeries != nil {
		t.Fatal("removed series left a stale selection")
	}
}

func TestKeyboardGraphNavigationAndFocus(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	ui.window.Canvas().Unfocus()
	ui.applyGraphRange(start, start.Add(100*time.Second))
	initial := graphHistoryState(ui)
	ui.window.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyRight})
	if !ui.view.Start.Equal(start.Add(25 * time.Second)) {
		t.Fatal("Right did not pan a quarter range")
	}
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	if graphHistoryState(ui) != initial {
		t.Fatal("keyboard pan cannot be undone")
	}
	ui.window.Canvas().OnTypedRune()('+')
	if ui.view.End.Sub(ui.view.Start) != 80*time.Second || !ui.view.Start.Equal(start.Add(10*time.Second)) {
		t.Fatal("keyboard zoom was not centered")
	}
	ui.window.Canvas().OnTypedRune()('-')
	if ui.view.End.Sub(ui.view.Start) != 100*time.Second {
		t.Fatal("keyboard zoom out did not restore range width")
	}
	key := chartSeriesKey{"a", ""}
	ui.outgoing.selectedSeries = &key
	ui.window.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if ui.outgoing.selectedSeries != nil {
		t.Fatal("Escape did not clear series emphasis")
	}
	before := graphHistoryState(ui)
	ui.window.Canvas().Focus(ui.start)
	ui.graphNavigationKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	ui.zoomGraphKeyboard(true)
	if graphHistoryState(ui) != before {
		t.Fatal("typing focus changed graph range")
	}
	ui.window.Canvas().Unfocus()
	popup := widget.NewPopUp(container.NewVBox(widget.NewLabel("Open menu")), ui.window.Canvas())
	popup.Show()
	ui.graphNavigationKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	popup.Hide()
	ui.tabs.SelectIndex(1)
	ui.zoomGraphKeyboard(true)
	if graphHistoryState(ui) != before {
		t.Fatal("popup or non-graph tab changed graph range")
	}
}

func TestEngagementKeyboardOpensHighlightedRow(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	viewer := ui.engagements
	viewer.initTable()
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	viewer.rows = []engagement{{Start: start, End: start.Add(10 * time.Minute), Summary: "First"}, {Start: start.Add(time.Hour), End: start.Add(2 * time.Hour), Summary: "Second"}}
	var opened []engagement
	viewer.onOpen = func(row engagement) { opened = append(opened, row) }
	viewer.table.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if len(opened) != 0 {
		t.Fatal("Enter on header opened an engagement")
	}
	viewer.table.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	viewer.table.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	viewer.table.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	viewer.table.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	if len(opened) != 2 || opened[0].Summary != "First" || opened[1].Summary != "Second" {
		t.Fatalf("Enter did not follow the keyboard highlight: %+v", opened)
	}
}
