package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) initESITable() {
	headings := [4]string{"Character", "Time Checked", "Ship", "Location"}
	ui.esiConnections = newCellSelectionTable(func() (int, int) { return len(ui.esiRows) + 1, len(headings) }, func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.Truncation = fyne.TextTruncateEllipsis
		return label
	}, func(id widget.TableCellID, object fyne.CanvasObject) {
		label := object.(*widget.Label)
		label.TextStyle.Bold = id.Row == 0
		if id.Row == 0 {
			label.SetText(headings[id.Col])
		} else {
			label.SetText(ui.esiRows[id.Row-1][id.Col])
		}
	}, func(object fyne.CanvasObject) string { return object.(*widget.Label).Text })
	ui.esiConnections.StickyRowCount = 1
	ui.esiConnections.StickyColumnCount = 1
}
