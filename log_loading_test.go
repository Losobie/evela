package main

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestLogFilenameDatesAndDefaultCutoff(t *testing.T) {
	for _, item := range []struct {
		name, want string
	}{
		{"20261018_025902_2123143905.txt", "2026-10-18"},
		{"20240229_235959_1.TXT", "2024-02-29"},
		{"20260229_025902_1.txt", ""},
		{"20261318_025902_1.txt", ""},
		{"20261032_025902_1.txt", ""},
		{"20261018backup.txt", ""},
		{"renamed.txt", ""},
		{"short", ""},
	} {
		date, ok := logFilenameDate(item.name)
		if ok != (item.want != "") || (ok && (date.Format("2006-01-02") != item.want || date.Location() != time.UTC)) {
			t.Fatalf("filename %q gave %s / %v", item.name, date, ok)
		}
	}
	// It is already October 18 UTC, even though the local calendar says October 17.
	now := time.Date(2026, 10, 17, 20, 59, 2, 0, time.FixedZone("Vancouver", -7*3600))
	if cutoff := logLoadingCutoff(now, defaultLogLookbackDays); !cutoff.Equal(time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("30-day cutoff must include all of September 18 UTC: %s", cutoff)
	}
	if !logLoadingCutoff(now, 0).IsZero() {
		t.Fatal("load-all unexpectedly has an age cutoff")
	}
}

func TestLogLoadingFiltersBeforeParsingAndPrunesCache(t *testing.T) {
	directory := t.TempDir()
	names := []string{
		"20260917_235959_1.txt", // Older than the inclusive default boundary.
		"20260918_000000_2.txt", // Exactly the boundary.
		"20261018_025902_3.TXT",
		"20270101_000000_4.txt", // Newer dates must also be loaded.
		"renamed.txt",
		"20260230_000000_6.txt", // Invalid date stays undated, rather than normalized.
	}
	for index, name := range names {
		path := filepath.Join(directory, name)
		// File contents intentionally all contain a much older damage event.
		content := fmt.Sprintf("Listener: Pilot %d\n[ 2000.01.01 00:00:00 ] (combat) 10 to Target - Hits\n", index)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		modified := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		if index == 0 {
			modified = time.Now() // A newly modified old filename must stay excluded.
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	index := &logIndex{}
	cutoff := logLoadingCutoff(time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC), defaultLogLookbackDays)
	progressTotal := 0
	snapshot, err := index.refresh(context.Background(), directory, cutoff, func(done, total int) {
		progressTotal = total
		if done > total {
			t.Fatal("progress exceeded included files")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Files != 3 || snapshot.SkippedOld != 1 || snapshot.SkippedUndated != 2 || len(snapshot.Characters) != 3 || len(index.files) != 3 || progressTotal != 3 || len(snapshot.Warnings) != 0 {
		t.Fatalf("filename filter did not limit parsing/cache/progress: %+v", snapshot)
	}
	if snapshot.First.Year() != 2000 {
		t.Fatal("filter incorrectly used the timestamp inside the log")
	}
	todayPath := filepath.Join(directory, names[2])
	original := index.files[todayPath].Log
	// Disabling the limit includes renamed exports; narrowing it frees cached logs.
	snapshot, err = index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil || snapshot.Files != 6 || len(index.files) != 6 {
		t.Fatalf("load-all did not restore excluded files: %+v / %v", snapshot, err)
	}
	snapshot, err = index.refresh(context.Background(), directory, cutoff.AddDate(0, 0, 1), nil)
	if err != nil || snapshot.Files != 2 || snapshot.SkippedOld != 2 || snapshot.SkippedUndated != 2 || len(index.files) != 2 || index.files[todayPath].Log != original {
		t.Fatalf("rolling cutoff did not prune old cache or reuse eligible files: %+v / %v", snapshot, err)
	}
	snapshot, err = index.refresh(context.Background(), directory, cutoff, nil)
	if err != nil || snapshot.Files != 3 || len(index.files) != 3 {
		t.Fatalf("widening the limit did not reread the newly eligible file: %+v / %v", snapshot, err)
	}
}

func TestLogLoadingSettingsSaveCancelValidateAndPersist(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Log loading settings")
	defer window.Close()
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1100, 800))
	window.Show()
	if ui.logLookbackDays != 30 {
		t.Fatal("new users do not default to the last 30 days")
	}
	open := func() (*widget.Entry, *widget.Check, *widget.Button) {
		t.Helper()
		for _, menu := range window.MainMenu().Items {
			for _, item := range menu.Items {
				if item.Label == "Log loading…" {
					item.Action()
				}
			}
		}
		overlay, ok := window.Canvas().Overlays().Top().(fyne.Widget)
		if !ok {
			t.Fatalf("Settings did not open the log loading dialog: %T", window.Canvas().Overlays().Top())
		}
		var popup *widget.PopUp
		for _, object := range test.WidgetRenderer(overlay).Objects() {
			if candidate, ok := object.(*widget.PopUp); ok {
				popup = candidate
			}
		}
		if popup == nil {
			t.Fatal("settings overlay has no dialog popup")
		}
		var days *widget.Entry
		var all *widget.Check
		var save *widget.Button
		walkFilterObjects(popup.Content, func(object fyne.CanvasObject) {
			switch value := object.(type) {
			case *widget.Form:
				days = value.Items[0].Widget.(*widget.Entry)
			case *widget.Check:
				all = value
			case *widget.Button:
				if value.Text == "Save and refresh" {
					save = value
				}
			}
		})
		if days == nil || all == nil || save == nil {
			t.Fatal("log loading settings are missing their controls")
		}
		return days, all, save
	}
	days, all, save := open()
	if days.Text != "30" || all.Checked || save.Disabled() {
		t.Fatal("default dialog does not show the 30-day limit")
	}
	if path := os.Getenv("EVELA_LOG_LOADING_CAPTURE"); path != "" {
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
	days.SetText("7")
	ui.logLoadingDialog.Hide() // Cancel must not change the saved limit or request a load.
	if ui.logLookbackDays != 30 || savedLogLookbackDays() != 30 || len(ui.loadRequests) != 0 {
		t.Fatal("cancelling changed settings or triggered loading")
	}
	days, all, save = open()
	for _, invalid := range []string{"", "0", "-1", "1.5", "forever", "36501"} {
		days.SetText(invalid)
		if !save.Disabled() {
			t.Fatalf("invalid day limit %q can be saved", invalid)
		}
	}
	days.SetText("7")
	test.Tap(save)
	if ui.logLookbackDays != 7 || savedLogLookbackDays() != 7 || ui.logLoadingDialog != nil || len(ui.loadRequests) != 1 {
		t.Fatal("saving did not persist the day limit and request an immediate refresh")
	}
	<-ui.loadRequests
	days, all, save = open()
	if days.Text != "7" {
		t.Fatal("reopening settings lost the saved value")
	}
	days.SetText("bad")
	all.SetChecked(true)
	if !days.Disabled() || save.Disabled() {
		t.Fatal("load-all still requires a valid day limit")
	}
	test.Tap(save)
	if ui.logLookbackDays != 0 || savedLogLookbackDays() != 0 || len(ui.loadRequests) != 1 {
		t.Fatal("load-all was not saved or applied")
	}
	secondWindow := application.NewWindow("Restored settings")
	defer secondWindow.Close()
	if restored := newAnalyzerUI(secondWindow); restored.logLookbackDays != 0 {
		t.Fatal("new UI did not load persisted settings")
	}
}
