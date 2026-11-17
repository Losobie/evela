package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"strings"
)

func (ui *analyzerUI) initESITable() {
	headings := [5]string{"Character", "Time Checked", "Ship", "Location", "ESI Status"}
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
	ui.esiConnections.onSecondary = func(id widget.TableCellID, position fyne.Position) {
		record := ui.ships[characterKey(ui.esiRows[id.Row-1][0])]
		menu := fyne.NewMenu("", fyne.NewMenuItem("Disconnect", func() {
			ui.requestESI(esiAction{Kind: "disconnect", CharacterID: record.CharacterID})
		}))
		widget.ShowPopUpMenuAtPosition(menu, ui.window.Canvas(), position)
	}
}

func (ui *analyzerUI) esiTableContent() *fyne.Container {
	return container.New(&fillingTableLayout{
		table: ui.esiConnections, widths: []float32{180, 210, 140, 420},
		maximumShares: []float32{0.15, 0.18, 0.12, 0.35},
	}, ui.esiConnections)
}

func characterESIStatus(record shipRecord) string {
	var issues []string
	for _, item := range []struct{ label, message string }{
		{"Ship", record.Error}, {"Ship class", record.ClassError},
		{"Location", record.Location.Error}, {"System name", record.Location.NameError},
		{"Station/structure name", record.Location.DockedNameError},
	} {
		if item.message != "" {
			issues = append(issues, item.label+": "+item.message)
		}
	}
	if len(issues) > 0 {
		return strings.Join(issues, " · ")
	}
	if record.CheckedAt.IsZero() && record.Location.CheckedAt.IsZero() {
		return "Not checked"
	}
	return "OK"
}
