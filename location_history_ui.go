package main

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) openLocationHistory() {
	logs := ui.logLocations.clone()
	logs.merge(ui.snapshot.locationSessions())
	content := container.NewAppTabs(container.NewTabItem("Game logs", logLocationHistoryContent(logs)), container.NewTabItem("ESI observations", locationHistoryContent(ui.locationHistory)))
	viewer := dialog.NewCustom("Location history", "Close", content, ui.window)
	viewer.Resize(fyne.NewSize(1140, 620))
	viewer.Show()
}

func locationHistoryContent(history locationHistory) fyne.CanvasObject {
	periods := history.periods()
	var observations int
	for _, period := range periods {
		observations += period.Observations
	}
	summary := widget.NewLabel(fmt.Sprintf("%d characters · %d observed periods · %d observations", len(history), len(periods), observations))
	note := widget.NewLabel("Recorded from successful ESI location queries. Consecutive sightings of the same location are combined; failures, gaps over 90 seconds and restarts begin a new period. The System filter assumes the last observed location for up to 45 seconds after its last sighting, until a newer location is observed. Listed timestamps remain actual observations. Close and reopen to see newer observations.")
	note.Wrapping = fyne.TextWrapWord
	top := container.NewVBox(summary, note, widget.NewSeparator())
	if len(periods) == 0 {
		return container.NewBorder(top, nil, nil, nil, widget.NewLabel("No ESI location history yet. Sign in with location access and refresh to start recording."))
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
				label.SetText([]string{"Character", "System", "System ID", "Station / structure ID", "First seen · EVE time", "Last seen · EVE time", "Samples"}[id.Col])
				return
			}
			period := periods[id.Row-1]
			system := systemFilterOption(period, true).Label
			docked := "In space"
			if period.StationID > 0 {
				docked = "Station " + strconv.FormatInt(period.StationID, 10)
			} else if period.StructureID > 0 {
				docked = "Structure " + strconv.FormatInt(period.StructureID, 10)
			}
			label.SetText([]string{period.CharacterName, system, strconv.FormatInt(period.SystemID, 10), docked,
				period.FirstSeen.UTC().Format("2006-01-02 15:04:05"), period.LastSeen.UTC().Format("2006-01-02 15:04:05"), strconv.Itoa(period.Observations)}[id.Col])
		})
	for col, width := range []float32{145, 110, 100, 190, 175, 175, 85} {
		table.SetColumnWidth(col, width)
	}
	table.StickyRowCount = 1
	return container.NewBorder(top, nil, nil, nil, table)
}
