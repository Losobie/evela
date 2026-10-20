package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func logLocationHistoryContent(history logLocationHistory) fyne.CanvasObject {
	periods := history.periods()
	note := widget.NewLabel("Jump and Conduit Field messages establish the destination from their timestamp through the remaining log session. Jump origins also identify the earlier part of that session. Game logs take precedence over ESI, including newer cached ESI responses. Coverage ends 45 seconds after the last timestamped entry; sessions are kept separate. The table shows session coverage before that allowance. Close and reopen to see newer locations.")
	note.Wrapping = fyne.TextWrapWord
	top := container.NewVBox(widget.NewLabel(fmt.Sprintf("%d log sessions · %d system periods", len(history), len(periods))), note, widget.NewSeparator())
	if len(periods) == 0 {
		return container.NewBorder(top, nil, nil, nil, widget.NewLabel("No log locations yet. Load logs containing jump or Conduit Field messages; ESI sign-in is optional."))
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
		label.SetText([]string{period.CharacterName, period.SystemName, period.FirstSeen.Format("2006-01-02 15:04:05"), period.LastSeen.Format("2006-01-02 15:04:05")}[id.Col])
	})
	for col, width := range []float32{220, 180, 260, 260} {
		table.SetColumnWidth(col, width)
	}
	table.StickyRowCount = 1
	return container.NewBorder(top, nil, nil, nil, table)
}
