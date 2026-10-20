package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// dateTimePicker keeps a precise, editable EVE timestamp and offers a calendar
// with 24-hour time selectors. Each selection updates the value immediately.
type dateTimePicker struct {
	widget.Entry
	popup                *widget.PopUp
	calendar             *widget.Calendar
	hour, minute, second *widget.Select
}

func newDateTimePicker() *dateTimePicker {
	picker := &dateTimePicker{}
	picker.ExtendBaseWidget(picker)
	picker.SetPlaceHolder("YYYY-MM-DD HH:MM:SS")
	picker.Validator = func(value string) error {
		_, err := parseRangeTime(value)
		return err
	}
	return picker
}

func (picker *dateTimePicker) CreateRenderer() fyne.WidgetRenderer {
	if picker.ActionItem == nil {
		picker.ActionItem = widget.NewButtonWithIcon("", theme.CalendarIcon(), picker.open)
	}
	return picker.Entry.CreateRenderer()
}

func (picker *dateTimePicker) open() {
	canvas := fyne.CurrentApp().Driver().CanvasForObject(picker)
	if canvas == nil {
		return
	}
	if picker.popup != nil {
		picker.popup.Hide()
	}
	at, err := parseRangeTime(picker.Text)
	if err != nil {
		at = time.Now().UTC().Truncate(time.Second)
	}
	setValue := func() { picker.SetText(at.Format("2006-01-02 15:04:05")) }
	picker.calendar = widget.NewCalendar(at, func(date time.Time) {
		at = time.Date(date.Year(), date.Month(), date.Day(), at.Hour(), at.Minute(), at.Second(), 0, time.UTC)
		setValue()
	})
	makeTimeSelect := func(count, selected int, changed func(int)) *widget.Select {
		options := make([]string, count)
		for index := range options {
			options[index] = fmt.Sprintf("%02d", index)
		}
		selectTime := widget.NewSelect(options, nil)
		selectTime.SetSelected(options[selected])
		selectTime.OnChanged = func(value string) {
			number, _ := strconv.Atoi(value) // Options contain only valid integers.
			changed(number)
			setValue()
		}
		return selectTime
	}
	picker.hour = makeTimeSelect(24, at.Hour(), func(hour int) {
		at = time.Date(at.Year(), at.Month(), at.Day(), hour, at.Minute(), at.Second(), 0, time.UTC)
	})
	picker.minute = makeTimeSelect(60, at.Minute(), func(minute int) {
		at = time.Date(at.Year(), at.Month(), at.Day(), at.Hour(), minute, at.Second(), 0, time.UTC)
	})
	picker.second = makeTimeSelect(60, at.Second(), func(second int) {
		at = time.Date(at.Year(), at.Month(), at.Day(), at.Hour(), at.Minute(), second, 0, time.UTC)
	})
	timeFields := container.NewGridWithColumns(3,
		container.NewVBox(widget.NewLabel("Hour"), picker.hour),
		container.NewVBox(widget.NewLabel("Minute"), picker.minute),
		container.NewVBox(widget.NewLabel("Second"), picker.second))
	close := widget.NewButton("Done", func() { picker.popup.Hide() })
	content := container.NewVBox(widget.NewLabel("EVE time · 24-hour clock"), picker.calendar, timeFields, close)
	picker.popup = widget.NewPopUp(content, canvas)
	picker.popup.ShowAtRelativePosition(fyne.NewPos(0, picker.Size().Height), picker)
}
