package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const selectDayOption = "Select day"

var quickRangeOptions = []string{"Last 10 minutes", "Last 30 minutes", "Last hour", "Last 24 hours", "Last 7 days", selectDayOption, "All history"}

func (ui *analyzerUI) quickRangeMenu() *fyne.Menu {
	menu := fyne.NewMenu("Quick Select")
	for _, option := range quickRangeOptions {
		option := option
		item := fyne.NewMenuItem(option, func() { ui.selectQuickRange(option) })
		item.Checked = ui.rangePreset == option
		menu.Items = append(menu.Items, item)
	}
	return menu
}

func (ui *analyzerUI) selectQuickRange(option string) {
	if option == selectDayOption {
		ui.openDayDialog()
		return
	}
	ui.clearGraphRangeHistory()
	ui.rangePreset = option
	ui.updateRange()
}

func (ui *analyzerUI) updateRangeLabel() {
	ui.rangeLabel.SetText(fmt.Sprintf("Time range: %s to %s EVE Time", ui.rangeStart.UTC().Format("2006-01-02 15:04:05"), ui.rangeEnd.UTC().Format("2006-01-02 15:04:05")))
}

func (ui *analyzerUI) selectDay(date time.Time) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	ui.clearGraphRangeHistory()
	ui.restoreGraphRange(graphRangeState{Start: start, End: start.AddDate(0, 0, 1), Preset: selectDayOption})
}

func (ui *analyzerUI) openDayDialog() {
	if ui.dayDialog != nil {
		ui.dayDialog.Show()
		return
	}
	date := ui.rangeStart
	if date.IsZero() {
		date = time.Now().UTC()
	}
	ui.dayCalendar = widget.NewCalendar(date.UTC(), func(selected time.Time) {
		ui.dayDialog.Hide()
		ui.selectDay(selected)
	})
	ui.dayDialog = dialog.NewCustom(selectDayOption, "Cancel", ui.dayCalendar, ui.window)
	ui.dayDialog.SetOnClosed(func() { ui.dayDialog = nil })
	ui.dayDialog.Show()
}
