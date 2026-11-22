package main

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"
)

const (
	closeActionPreference = "window.closeAction"
	closeActionMinimize   = "minimize"
	closeActionQuit       = "quit"
)

func (ui *analyzerUI) setupSystemTray(application fyne.App, initializeWindow func()) bool {
	tray, ok := application.(desktop.App)
	if !ok {
		return false
	}

	exit := fyne.NewMenuItem("Exit", application.Quit)
	exit.IsQuit = true
	show := func() {
		ui.window.Show()
		initializeWindow()
		ui.resumeForegroundWork()
	}
	tray.SetSystemTrayMenu(fyne.NewMenu("Evela",
		fyne.NewMenuItem("Show Evela", show),
		fyne.NewMenuItemSeparator(),
		exit,
	))
	// Route both tray actions through the same restore path.
	systray.SetOnTapped(func() { fyne.Do(show) })

	window := ui.window
	minimize := func() {
		ui.pauseForegroundWork()
		window.Hide()
	}
	prefs := application.Preferences()
	var closePrompt *dialog.CustomDialog
	window.SetCloseIntercept(func() {
		if closePrompt != nil {
			return
		}
		switch prefs.String(closeActionPreference) {
		case closeActionMinimize:
			minimize()
			return
		case closeActionQuit:
			application.Quit()
			return
		}

		remember := widget.NewCheck("Remember my choice", nil)
		prompt := dialog.NewCustomWithoutButtons("Close Evela", container.NewVBox(
			widget.NewLabel("Minimize Evela to the system tray or close the application?"),
			remember,
		), window)
		prompt.SetOnClosed(func() { closePrompt = nil })
		choose := func(action string, run func()) func() {
			return func() {
				if remember.Checked {
					prefs.SetString(closeActionPreference, action)
				}
				prompt.Hide()
				run()
			}
		}
		prompt.SetButtons([]fyne.CanvasObject{
			widget.NewButton("Cancel", prompt.Hide),
			widget.NewButton("Minimize to tray", choose(closeActionMinimize, minimize)),
			widget.NewButton("Close application", choose(closeActionQuit, application.Quit)),
		})
		closePrompt = prompt
		prompt.Show()
	})
	return true
}

// The UI thread owns the foreground context and cancels it when the window
// goes to the tray. Workers wait on the ready channel until it is restored.
func (ui *analyzerUI) pauseForegroundWork() {
	ui.foregroundReady = make(chan struct{})
	ui.cancelForeground()
	ui.engagements.cancelBuild()
}

func (ui *analyzerUI) resumeForegroundWork() {
	if ui.foregroundReady == nil {
		return
	}
	ui.foregroundCtx, ui.cancelForeground = context.WithCancel(context.Background())
	close(ui.foregroundReady)
	ui.foregroundReady = nil
	if ui.auto.Checked {
		ui.requestLoad()
	}
	if update := ui.updatePendingESIView; update != nil {
		ui.updatePendingESIView = nil
		update()
	}
	ui.refreshEngagements()
}

func (ui *analyzerUI) waitForForeground() context.Context {
	for {
		select {
		case <-ui.stop:
			return nil
		default:
		}
		var ctx context.Context
		var ready <-chan struct{}
		fyne.DoAndWait(func() {
			ctx, ready = ui.foregroundCtx, ui.foregroundReady
		})
		if ready == nil {
			return ctx
		}
		select {
		case <-ui.stop:
			return nil
		case <-ready:
		}
	}
}
