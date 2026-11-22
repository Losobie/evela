package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const startInTrayArg = "--start-in-tray"

func (ui *analyzerUI) openStartupTraySettings() {
	prefs := fyne.CurrentApp().Preferences()
	minimizeOnClose := widget.NewCheck("Minimize to system tray", nil)
	minimizeOnClose.SetChecked(prefs.String(closeActionPreference) == closeActionMinimize)
	note := widget.NewLabel("Save with this unchecked to restore the window close prompt.")
	note.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(widget.NewLabel("When closing the window"), minimizeOnClose, note)
	startup, saveStartup := windowsStartupControls()
	if startup != nil {
		content = container.NewVBox(startup, widget.NewSeparator(), content)
	}
	settings := dialog.NewCustomWithoutButtons("Startup & Tray", content, ui.window)
	settings.Resize(fyne.NewSize(450, 240))
	settings.SetButtons([]fyne.CanvasObject{
		widget.NewButton("Cancel", settings.Hide),
		widget.NewButton("Save", func() {
			if saveStartup != nil {
				if err := saveStartup(); err != nil {
					reportRuntimeError("Save Windows startup settings", err)
					dialog.ShowError(err, ui.window)
					return
				}
			}
			if minimizeOnClose.Checked {
				prefs.SetString(closeActionPreference, closeActionMinimize)
			} else {
				prefs.RemoveValue(closeActionPreference)
			}
			settings.Hide()
		}),
	})
	settings.Show()
}
