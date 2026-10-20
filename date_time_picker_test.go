package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestDateTimePickerAppliesRangeChanges(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	test.Tap(ui.start.ActionItem.(*widget.Button))
	if ui.start.popup == nil || !ui.start.popup.Visible() || ui.start.hour.Selected != "02" {
		t.Fatal("calendar action did not open a picker at the current timestamp")
	}
	ui.start.minute.SetSelected("30")
	assertGraphHistoryState(t, ui, graphRangeState{Start: start.Add(30 * time.Minute), End: start.Add(time.Hour), Preset: "Custom"}, 180)
	ui.start.second.SetSelected("01")
	assertGraphHistoryState(t, ui, graphRangeState{Start: start.Add(30*time.Minute + time.Second), End: start.Add(time.Hour), Preset: "Custom"}, 150)
	ui.start.popup.Hide()
	test.Tap(ui.end.ActionItem.(*widget.Button))
	ui.end.hour.SetSelected("02")
	ui.end.minute.SetSelected("46")
	assertGraphHistoryState(t, ui, graphRangeState{Start: start.Add(30*time.Minute + time.Second), End: start.Add(46 * time.Minute), Preset: "Custom"}, 60)
	ui.end.popup.Hide()
	// No Apply button or explicit rebuild is needed, and the saved range follows
	// both the calendar and time controls.
	if path := os.Getenv("EVELA_PICKER_CAPTURE"); path != "" {
		test.Tap(ui.end.ActionItem.(*widget.Button))
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, ui.window.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}
	restored := newRangePreferenceTestUI(t, fyne.CurrentApp(), ui.snapshot)
	if graphHistoryState(restored) != graphHistoryState(ui) {
		t.Fatal("picker changes did not save their exact bounds")
	}
}

func TestDateTimePickerCalendarAndInvalidRanges(t *testing.T) {
	ui, start := newGraphHistoryTestUI(t)
	test.Tap(ui.end.ActionItem.(*widget.Button))
	// Select the following day through the actual calendar button.
	var nextDay *widget.Button
	for _, object := range test.WidgetRenderer(ui.end.calendar).Objects() {
		walkFilterObjects(object, func(object fyne.CanvasObject) {
			if button, ok := object.(*widget.Button); ok && button.Text == "18" {
				nextDay = button
			}
		})
	}
	if nextDay == nil {
		t.Fatal("calendar did not contain the next day's button")
	}
	test.Tap(nextDay)
	wantEnd := start.Add(25 * time.Hour)
	if !ui.view.End.Equal(wantEnd) || ui.view.End.Location() != time.UTC {
		t.Fatalf("calendar did not preserve the EVE time of day: %v", ui.view.End)
	}
	ui.end.popup.Hide()
	want := graphHistoryState(ui)
	saved := fyne.CurrentApp().Preferences().String(graphRangePreference)
	for _, invalid := range []string{"unfinished date", ui.start.Text, start.Add(-time.Second).Format("2006-01-02 15:04:05")} {
		ui.end.SetText(invalid)
		if ui.end.Validate() == nil || graphHistoryState(ui) != want || fyne.CurrentApp().Preferences().String(graphRangePreference) != saved {
			t.Fatal("an invalid picker range replaced the applied or saved range")
		}
	}
	ui.end.SetText(start.Add(time.Hour).Format("2006-01-02 15:04:05"))
	if ui.end.Validate() != nil || !ui.view.End.Equal(start.Add(time.Hour)) {
		t.Fatal("repairing an invalid range did not apply it immediately")
	}
	// Presets keep following the logs after a picker edit and initialise the
	// popup from the refreshed bounds rather than the previous custom range.
	ui.rangeSelect.SetSelected("Last 30 minutes")
	ui.snapshot.Last = ui.snapshot.Last.Add(24 * time.Hour)
	ui.updateRange()
	test.Tap(ui.end.ActionItem.(*widget.Button))
	if ui.end.hour.Selected != "03" || ui.view.End != start.Add(25*time.Hour) || ui.rangeSelect.Selected != "Last 30 minutes" {
		t.Fatal("opening the picker changed the preset or used stale bounds")
	}
}
