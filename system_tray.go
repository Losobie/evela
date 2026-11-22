package main

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"
)

func (ui *analyzerUI) setupSystemTray(application fyne.App) {
	tray, ok := application.(desktop.App)
	if !ok {
		return
	}

	quit := fyne.NewMenuItem("Quit", application.Quit)
	quit.IsQuit = true
	show := func() {
		ui.window.Show()
		ui.resumeForegroundWork()
	}
	tray.SetSystemTrayMenu(fyne.NewMenu("Evela",
		fyne.NewMenuItem("Show Evela", show),
		fyne.NewMenuItemSeparator(),
		quit,
	))
	// Route both tray actions through the same restore path.
	systray.SetOnTapped(func() { fyne.Do(show) })

	window := ui.window
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
				ui.pauseForegroundWork()
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
