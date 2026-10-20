package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// Full refresh diagnostics remain available without taking space in the main view.
func (ui *analyzerUI) showRefreshDetails(title, message string) {
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	label.Selectable = true
	details := dialog.NewCustom(title, "Close", container.NewVScroll(label), ui.window)
	details.Resize(fyne.NewSize(720, 320))
	details.Show()
}

func (ui *analyzerUI) esiStatusRow(label *widget.Label) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil,
		widget.NewButton("Details…", func() { ui.showRefreshDetails("ESI status", label.Text) }), label)
}
