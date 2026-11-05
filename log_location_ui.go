package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func logLocationHistoryContent(history logLocationHistory) fyne.CanvasObject {
	periods := history.periods()
	note := widget.NewLabel("Jump and Conduit Field messages establish the destination through the remaining log session and take precedence over ESI. A spatial-phenomena notification with no jump in the preceding 30 seconds provides an assumed Wormhole location; ESI takes priority over that assumption. Coverage ends 45 seconds after the last timestamped entry; sessions are kept separate. The table shows log evidence before that allowance. Close and reopen to see newer locations.")
	note.Wrapping = fyne.TextWrapWord
	top := container.NewVBox(widget.NewLabel(fmt.Sprintf("%d log sessions · %d system periods", len(history), len(periods))), note, widget.NewSeparator())
	if len(periods) == 0 {
		return container.NewBorder(top, nil, nil, nil, widget.NewLabel("No log locations yet. Load logs containing jump, Conduit Field or spatial-phenomena messages; ESI sign-in is optional."))
	}
	table := widget.NewTable(func() (int, int) { return len(periods) + 1, 4 }, func() fyne.CanvasObject {
		label := widget.NewLabel("")
		label.Selectable, label.Truncation = true, fyne.TextTruncateEllipsis
		return label
	}, func(id widget.TableCellID, object fyne.CanvasObject) {
		label := object.(*widget.Label)
		label.TextStyle.Bold = id.Row == 0
		if id.Row == 0 {
			label.SetText([]string{"Character", "System", "Coverage start · EVE time", "Coverage end · EVE time"}[id.Col])
			return
		}
		period := periods[id.Row-1]
		system := period.SystemName
		if period.Source == inferredLogLocationSource {
			system += " (assumed)"
		}
		label.SetText([]string{period.CharacterName, system, period.FirstSeen.Format("2006-01-02 15:04:05"), period.LastSeen.Format("2006-01-02 15:04:05")}[id.Col])
	})
	for col, width := range []float32{220, 180, 260, 260} {
		table.SetColumnWidth(col, width)
	}
	table.StickyRowCount = 1
	return container.NewBorder(top, nil, nil, nil, table)
}
