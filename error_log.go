package main

import (
	"bytes"
	"io"
	"log"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// Runtime diagnostics stay in memory for the current application session.
type runtimeErrorLog struct {
	mu   sync.Mutex
	text bytes.Buffer
}

var runtimeErrors runtimeErrorLog

func (store *runtimeErrorLog) Write(data []byte) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.text.Write(data)
}

func (store *runtimeErrorLog) Text() string {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.text.String()
}

func captureRuntimeErrors() {
	// GUI builds may have no usable stderr handle. Capture before forwarding.
	log.SetOutput(io.MultiWriter(&runtimeErrors, log.Writer()))
}

func reportRuntimeError(context string, err error) {
	if err != nil {
		log.Printf("%s: %v", context, err)
	}
}

func newErrorLogDialog(window fyne.Window, store *runtimeErrorLog, diagnostics func() string) *dialog.CustomDialog {
	text := widget.NewLabel("")
	text.Wrapping, text.Selectable = fyne.TextWrapWord, true
	refresh := func() {
		value := store.Text()
		if value == "" {
			value = "No errors recorded during this session."
		}
		if diagnostics != nil {
			value = "Diagnostics report\n" + diagnostics() + "\nSession errors\n" + value
		}
		text.SetText(value)
	}
	refresh()
	copyAll := widget.NewButton("Copy all", func() {
		refresh()
		window.Clipboard().SetContent(text.Text)
	})
	controls := container.NewHBox(widget.NewButton("Refresh", refresh), copyAll)
	content := container.NewBorder(nil, controls, nil, nil, container.NewVScroll(text))
	viewer := dialog.NewCustom("Error log", "Close", content, window)
	viewer.Resize(fyne.NewSize(900, 520))
	return viewer
}

func (ui *analyzerUI) openErrorLog() {
	if ui.errorLogDialog != nil {
		ui.errorLogDialog.Show()
		return
	}
	ui.errorLogDialog = newErrorLogDialog(ui.window, &runtimeErrors, ui.diagnosticsReport)
	ui.errorLogDialog.SetOnClosed(func() { ui.errorLogDialog = nil })
	ui.errorLogDialog.Show()
}
