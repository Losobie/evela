package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

func setupSystemTray(application fyne.App, window fyne.Window) {
	tray, ok := application.(desktop.App)
	if !ok {
		return
	}

	quit := fyne.NewMenuItem("Quit", application.Quit)
	quit.IsQuit = true
	tray.SetSystemTrayMenu(fyne.NewMenu("Evela",
		fyne.NewMenuItem("Show Evela", window.Show),
		fyne.NewMenuItemSeparator(),
		quit,
	))
	// Fyne installs its own hide-on-close handler here. Install the prompt after it.
	tray.SetSystemTrayWindow(window)

	var closePrompt *dialog.CustomDialog
	window.SetCloseIntercept(func() {
		if closePrompt != nil {
			return
		}

		prompt := dialog.NewCustomWithoutButtons("Close Evela",
			widget.NewLabel("Minimize Evela to the system tray or close the application?"), window)
		prompt.SetOnClosed(func() { closePrompt = nil })
		prompt.SetButtons([]fyne.CanvasObject{
			widget.NewButton("Cancel", prompt.Hide),
			widget.NewButton("Minimize to tray", func() {
				prompt.Hide()
				window.Hide()
			}),
			widget.NewButton("Close application", func() {
				prompt.Hide()
				application.Quit()
			}),
		})
		closePrompt = prompt
		prompt.Show()
	})
}
