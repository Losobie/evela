package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Keep the first two columns at their preferred widths, shrinking them on
// narrow windows, and give the last column the remaining viewport width.
// Cell labels truncate instead of creating a horizontal scrollbar.
func newFillingTable(table *widget.Table, first, second float32) *fyne.Container {
	return container.New(&fillingTableLayout{table: table, first: first, second: second}, table)
}

type fillingTableLayout struct {
	table         *widget.Table
	first, second float32
}

func (l *fillingTableLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(400, l.table.MinSize().Height)
}

func (l *fillingTableLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	available := max(0, size.Width-2*l.table.Theme().Size(theme.SizeNamePadding))
	first, second := min(l.first, available*0.4), min(l.second, available*0.2)
	l.table.SetColumnWidth(0, first)
	l.table.SetColumnWidth(1, second)
	l.table.SetColumnWidth(2, max(0, available-first-second))
	l.table.Move(fyne.NewPos(0, 0))
	l.table.Resize(size)
}
