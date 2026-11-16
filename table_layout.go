package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// Keep the leading columns at their preferred widths, shrinking them on
// narrow windows, and give the last column the remaining viewport width.
// Cell labels truncate instead of creating a horizontal scrollbar.
type fillingTable interface {
	fyne.CanvasObject
	SetColumnWidth(int, float32)
	Theme() fyne.Theme
}

func newFillingTable(table fillingTable, widths ...float32) *fyne.Container {
	return container.New(&fillingTableLayout{table: table, widths: widths}, table)
}

type fillingTableLayout struct {
	table  fillingTable
	widths []float32
}

func (l *fillingTableLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, l.table.MinSize().Height)
}

func (l *fillingTableLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	available := max(0, size.Width-float32(len(l.widths))*l.table.Theme().Size(theme.SizeNamePadding))
	remaining := available
	// Reserve at least 40% for the final column, with twice as much space
	// available to the first column as to each other leading column.
	unit := available * 0.6 / float32(len(l.widths)+1)
	for col, preferred := range l.widths {
		limit := unit
		if col == 0 {
			limit *= 2
		}
		width := min(preferred, limit)
		l.table.SetColumnWidth(col, width)
		remaining -= width
	}
	l.table.SetColumnWidth(len(l.widths), max(0, remaining))
	l.table.Move(fyne.NewPos(0, 0))
	l.table.Resize(size)
}
