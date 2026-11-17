package main

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) initESITable() {
	headings := [6]string{"Character", "Time Checked", "Ship", "Location", "Scopes", "ESI Status"}
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
		table: ui.esiConnections, widths: []float32{180, 200, 110, 330, 180},
		maximumShares: []float32{0.14, 0.16, 0.09, 0.26, 0.15},
	}, ui.esiConnections)
}

func characterESIScopes(scopes []string) string {
	if scopes == nil {
		return "Unknown"
	}
	var names []string
	for _, item := range []struct{ scope, name string }{{shipScope, "Ship"}, {locationScope, "Location"}, {structureScope, "Structures"}} {
		if slices.Contains(scopes, item.scope) {
			names = append(names, item.name)
		}
	}
	if len(names) == 3 {
		return "All"
	}
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, ", ")
}

func characterESIStatus(record shipRecord) string {
	var issues []string
	missingOnly := record.Scopes != nil && len(record.Scopes) == 0
	for _, item := range []struct{ label, message string }{
		{"Ship", record.Error}, {"Ship class", record.ClassError},
		{"Location", record.Location.Error}, {"System name", record.Location.NameError},
		{"Station/structure name", record.Location.DockedNameError},
	} {
		if item.message == shipScopeMissing || item.message == locationScopeMissing || item.message == structureScopeMissing {
			missingOnly = true
			continue
		}
		if item.message != "" {
			issues = append(issues, item.label+": "+item.message)
		}
	}
	if len(issues) > 0 {
		return strings.Join(issues, " · ")
	}
	if missingOnly {
		return "OK"
	}
	if record.CheckedAt.IsZero() && record.Location.CheckedAt.IsZero() {
		return "Not checked"
	}
	return "OK"
}
