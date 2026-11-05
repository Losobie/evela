package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestQuickSelectCalendarAndRangeLabel(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	menu := ui.quickRangeMenu()
	var labels []string
	for _, item := range menu.Items {
		labels = append(labels, item.Label)
	}
	want := []string{"Last 10 minutes", "Last 30 minutes", "Last hour", "Last 24 hours", "Last 7 days", "Select day", "All history"}
	if ui.quickSelect.Text != "Quick Select" || !reflect.DeepEqual(labels, want) {
		t.Fatalf("unexpected quick selections: %v", labels)
	}
	before := graphHistoryState(ui)
	menu.Items[5].Action()
	if ui.dayDialog == nil || graphHistoryState(ui) != before {
		t.Fatal("opening the calendar changed the current range")
	}
	var nextDay *widget.Button
	for _, object := range test.WidgetRenderer(ui.dayCalendar).Objects() {
		walkFilterObjects(object, func(object fyne.CanvasObject) {
			if button, ok := object.(*widget.Button); ok && button.Text == "18" {
				nextDay = button
			}
		})
	}
	if nextDay == nil {
		t.Fatal("calendar did not open at the current range's month")
	}
	if path := os.Getenv("EVELA_DAY_CALENDAR_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path)
	}
	test.Tap(nextDay)
	day := time.Date(start.Year(), start.Month(), 18, 0, 0, 0, 0, time.UTC)
	if !ui.view.Start.Equal(day) || !ui.view.End.Equal(day.AddDate(0, 0, 1)) || ui.dayDialog != nil || ui.rangePreset != selectDayOption {
		t.Fatalf("calendar did not apply a complete EVE day: %+v", graphHistoryState(ui))
	}
	if ui.rangeLabel.Text != "Time range: 2026-10-18 00:00:00 to 2026-10-19 00:00:00 EVE Time" {
		t.Fatalf("incorrect label: %q", ui.rangeLabel.Text)
	}
	ui.snapshot.Last = ui.snapshot.Last.Add(48 * time.Hour)
	ui.updateRange()
	if !ui.view.Start.Equal(day) {
		t.Fatal("refresh moved the selected day")
	}
	restored := newRangePreferenceTestUI(t, fyne.CurrentApp(), ui.snapshot)
	if graphHistoryState(restored) != graphHistoryState(ui) {
		t.Fatal("selected day was not restored")
	}
	ui.openDayDialog()
	ui.dayDialog.Hide()
	if !ui.view.Start.Equal(day) {
		t.Fatal("canceling calendar changed the range")
	}
	ui.applyGraphRange(day.Add(time.Hour), day.Add(2*time.Hour))
	if ui.rangeLabel.Text != "Time range: 2026-10-18 01:00:00 to 2026-10-18 02:00:00 EVE Time" {
		t.Fatal("zoom did not update the label")
	}
	ui.undoGraphRange()
	if ui.rangePreset != selectDayOption || !ui.view.Start.Equal(day) {
		t.Fatal("undo did not restore the selected day")
	}
	if path := os.Getenv("EVELA_QUICK_RANGE_CAPTURE"); path != "" {
		captureWindowPreview(t, ui.window, path)
	}
}

func TestSelectDayUsesEVECalendarDate(t *testing.T) {
	ui, _ := newGraphHistoryTestUI(t)
	for _, day := range []time.Time{
		time.Date(2024, 2, 29, 22, 30, 0, 0, time.FixedZone("calendar", -10*3600)),
		time.Date(2026, 12, 31, 14, 0, 0, 0, time.FixedZone("calendar", 12*3600)),
	} {
		ui.selectDay(day)
		if ui.view.Start.Location() != time.UTC || ui.view.Start.Day() != day.Day() || ui.view.Start.Hour() != 0 || ui.view.End.Sub(ui.view.Start) != 24*time.Hour {
			t.Fatal("day selection changed the chosen date or used local-time boundaries")
		}
	}
}
