package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	defaultLogLookbackDays = 30
	maxLogLookbackDays     = 36500
	logLookbackPreference  = "logs.lookback-days"
)

// EVE names sessions YYYYMMDD_HHMMSS_characterID.txt. Filename dates are
// interpreted as UTC calendar dates, independently of modification times.
func logFilenameDate(name string) (time.Time, bool) {
	if len(name) < 9 || name[8] != '_' {
		return time.Time{}, false
	}
	date, err := time.Parse("20060102", name[:8])
	return date, err == nil
}

func logLoadingCutoff(now time.Time, days int) time.Time {
	if days == 0 {
		return time.Time{} // Explicitly load all files, including undated exports.
	}
	year, month, day := now.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -days)
}

func savedLogLookbackDays() int {
	days := fyne.CurrentApp().Preferences().IntWithFallback(logLookbackPreference, defaultLogLookbackDays)
	if days < 0 || days > maxLogLookbackDays {
		return defaultLogLookbackDays
	}
	return days
}

func parseLogLookbackDays(value string) (int, error) {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || days < 1 || days > maxLogLookbackDays {
		return 0, fmt.Errorf("enter a whole number of days from 1 to %d", maxLogLookbackDays)
	}
	return days, nil
}

func (ui *analyzerUI) openLogLoadingSettings() {
	if ui.logLoadingDialog != nil {
		ui.logLoadingDialog.Show()
		return
	}
	note := widget.NewLabel("Load recent logs to reduce startup time and memory use. Dates come from filenames: 20261018_025902_2123143905.txt is October 18, 2026. File modification times and damage timestamps do not control this limit.")
	note.Wrapping = fyne.TextWrapWord
	folder := widget.NewMultiLineEntry()
	folder.SetPlaceHolder("One log folder per line, including local folders or \\\\computer\\share\\gamelogs")
	folder.SetMinRowsVisible(4)
	folder.SetText(ui.path.Text)
	browse := widget.NewButtonWithIcon("Add folder…", theme.FolderOpenIcon(), func() {
		dialog.NewFolderOpen(func(selected fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			if selected != nil {
				folder.SetText(strings.TrimSpace(folder.Text) + "\n" + selected.Path())
			}
		}, ui.window).Show()
	})
	days := widget.NewEntry()
	days.SetText(strconv.Itoa(ui.logLookbackDays))
	if ui.logLookbackDays == 0 {
		days.SetText(strconv.Itoa(defaultLogLookbackDays))
	}
	days.Validator = func(value string) error { _, err := parseLogLookbackDays(value); return err }
	all := widget.NewCheck("Load all log files", nil)
	all.SetChecked(ui.logLookbackDays == 0)
	preview := widget.NewLabel("")
	preview.Wrapping = fyne.TextWrapWord
	apply := func() {
		folders, err := parseLogFolders(folder.Text)
		if err != nil {
			return
		}
		limit := 0
		if !all.Checked {
			var err error
			limit, err = parseLogLookbackDays(days.Text)
			if err != nil {
				return
			}
		}
		ui.logLookbackDays = limit
		ui.path.SetText(strings.Join(folders, "\n"))
		saveLogFolders(folders)
		fyne.CurrentApp().Preferences().SetInt(logLookbackPreference, limit)
		ui.logLoadingDialog.Hide()
		ui.requestLoad()
	}
	save := widget.NewButton("Save and refresh", apply)
	update := func() {
		valid := true
		if all.Checked {
			days.Disable()
			preview.SetText("Load every .txt log, including files without a recognized filename date.")
		} else {
			days.Enable()
			limit, err := parseLogLookbackDays(days.Text)
			if err != nil {
				preview.SetText(err.Error())
				valid = false
			} else {
				preview.SetText("Load files dated " + logLoadingCutoff(time.Now(), limit).Format("January 2, 2006") + " or later (EVE time), including newer dates. Older files and files without a valid YYYYMMDD_ prefix are skipped before opening. This does not delete any logs.")
			}
		}
		if _, err := parseLogFolders(folder.Text); err != nil {
			preview.SetText(err.Error())
			valid = false
		}
		if valid {
			save.Enable()
		} else {
			save.Disable()
		}
	}
	all.OnChanged = func(bool) { update() }
	days.OnChanged = func(string) { update() }
	folder.OnChanged = func(string) { update() }
	update()
	folderNote := widget.NewLabel("Watch every listed folder together. Add a local folder and a network share, for example \\\\computer\\share\\gamelogs. Edit or delete a line to change or remove a folder. Unavailable folders are retried automatically; their last loaded data stays available. Duplicate exports across folders are counted once.")
	folderNote.Wrapping = fyne.TextWrapWord
	pathRow := container.NewBorder(nil, nil, widget.NewLabel("Log folders"), browse)
	content := container.NewVBox(pathRow, folder, folderNote, widget.NewSeparator(), note, widget.NewForm(widget.NewFormItem("Look back (days)", days)), all, preview, save)
	ui.logLoadingDialog = dialog.NewCustom("Log loading", "Cancel", content, ui.window)
	ui.logLoadingDialog.SetOnClosed(func() { ui.logLoadingDialog = nil })
	ui.logLoadingDialog.Resize(fyne.NewSize(860, 640))
	ui.logLoadingDialog.Show()
}
