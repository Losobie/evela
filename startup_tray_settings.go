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
	minimizeOnClose.OnChanged = func(checked bool) {
		if checked {
			prefs.SetString(closeActionPreference, closeActionMinimize)
		} else {
			prefs.RemoveValue(closeActionPreference)
		}
	}
	content := container.NewVBox(widget.NewLabel("When closing the window"), minimizeOnClose)
	if startup := windowsStartupControls(ui.window); startup != nil {
		content = container.NewVBox(startup, widget.NewSeparator(), content)
	}
	dialog.ShowCustom("Startup & Tray", "Close", content, ui.window)
}
