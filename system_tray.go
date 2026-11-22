package main

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"
)

const (
	closeActionPreference = "window.closeAction"
	closeActionMinimize   = "minimize"
	closeActionQuit       = "quit"
)

func (ui *analyzerUI) setupSystemTray(application fyne.App, initializeWindow func()) (minimize, stopListening func()) {
	tray, ok := application.(desktop.App)
	if !ok {
		return nil, nil
	}

	var stopRestore func()
	stopListening = func() {
		if stopRestore != nil {
			stopRestore()
			stopRestore = nil
		}
	}
	quit := func() {
		stopListening()
		application.Quit()
	}
	exit := fyne.NewMenuItem("Exit", quit)
	exit.IsQuit = true
	show := func() {
		stopListening()
		ui.window.Show()
		initializeWindow()
		ui.window.RequestFocus()
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
	minimize = func() {
		ui.pauseForegroundWork()
		window.Hide()
		stopRestore = listenForTrayRestore(func() {
			fyne.Do(func() {
				if stopRestore != nil {
					show()
				}
			})
		})
	}
	prefs := application.Preferences()
	var closePrompt fyne.Window
	window.SetCloseIntercept(func() {
		if closePrompt != nil {
			closePrompt.RequestFocus()
			return
		}
		switch prefs.String(closeActionPreference) {
		case closeActionMinimize:
			minimize()
			return
		case closeActionQuit:
			quit()
			return
		}

		remember := widget.NewCheck("Remember my choice", nil)
		prompt := application.NewWindow("Close Evela")
		prompt.SetOnClosed(func() { closePrompt = nil })
		choose := func(action string, run func()) func() {
			return func() {
				if remember.Checked {
					prefs.SetString(closeActionPreference, action)
				}
				prompt.Close()
				run()
			}
		}
		prompt.SetContent(container.NewPadded(container.NewVBox(
			widget.NewLabel("Minimize Evela to the system tray or close the application?"),
			remember,
			container.NewGridWithColumns(3,
				widget.NewButton("Cancel", prompt.Close),
				widget.NewButton("Minimize to tray", choose(closeActionMinimize, minimize)),
				widget.NewButton("Close application", choose(closeActionQuit, quit)),
			),
		)))
		prompt.SetFixedSize(true)
		prompt.CenterOnScreen()
		closePrompt = prompt
		prompt.Show()
		prompt.RequestFocus()
	})
	return minimize, stopListening
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
