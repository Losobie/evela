package main

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) openShipHistory() {
	viewer := dialog.NewCustom("Ship history", "Close", shipHistoryContent(ui.shipHistory), ui.window)
	viewer.Resize(fyne.NewSize(1140, 620))
	viewer.Show()
}

func shipHistoryContent(history shipHistory) fyne.CanvasObject {
	periods := history.periods()
	var observations int
	for _, period := range periods {
		observations += period.Observations
	}
	summary := widget.NewLabel(fmt.Sprintf("%d characters · %d observed periods · %d observations", len(history), len(periods), observations))
	note := widget.NewLabel("Recorded from successful ESI ship queries. Consecutive sightings of the same ship are combined; failures, gaps over 90 seconds and restarts begin a new period. Graphs assume the last observed ship for up to 45 seconds after its last sighting, until a newer ship is observed. Listed timestamps remain actual observations. Close and reopen to see newer observations.")
	note.Wrapping = fyne.TextWrapWord
	top := container.NewVBox(summary, note, widget.NewSeparator())
	if len(periods) == 0 {
		return container.NewBorder(top, nil, nil, nil, widget.NewLabel("No ship history yet. Connect a character and refresh ships to start recording."))
	}
	table := widget.NewTable(
		func() (int, int) { return len(periods) + 1, 7 },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			label.Selectable = true
			return label
		},
		func(id widget.TableCellID, object fyne.CanvasObject) {
			label := object.(*widget.Label)
			label.TextStyle.Bold = id.Row == 0
			if id.Row == 0 {
				label.SetText([]string{"Character", "Ship type", "Ship name", "Ship item ID", "First seen · EVE time", "Last seen · EVE time", "Samples"}[id.Col])
				return
			}
			period := periods[id.Row-1]
			typeName := period.TypeName
			if typeName == "" {
				typeName = "Type " + strconv.FormatInt(period.TypeID, 10)
			}
			shipID := strconv.FormatInt(period.ShipID, 10)
			if period.ShipID == 0 {
				shipID = "Unknown"
			}
			label.SetText([]string{period.CharacterName, typeName, period.ShipName, shipID,
				period.FirstSeen.UTC().Format("2006-01-02 15:04:05"), period.LastSeen.UTC().Format("2006-01-02 15:04:05"), strconv.Itoa(period.Observations)}[id.Col])
		})
	for col, width := range []float32{155, 105, 155, 140, 175, 175, 85} {
		table.SetColumnWidth(col, width)
	}
	table.StickyRowCount = 1
	return container.NewBorder(top, nil, nil, nil, table)
}
