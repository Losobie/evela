package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Keep Add filter at the left and the graph controls at the right of the
// first row. Filter chips use the remaining space, then wrap onto full rows.
type filterBarLayout struct {
	owner    *fyne.Container
	trailing fyne.CanvasObject
	width    float32
}

func (l *filterBarLayout) arrange(objects []fyne.CanvasObject, width float32, place bool) fyne.Size {
	gap := theme.Padding()
	var minWidth, height, x, y, rowHeight float32
	rightWidth := float32(0)
	if l.trailing != nil && l.trailing.Visible() {
		m := l.trailing.MinSize()
		rightWidth = m.Width + gap
		rowHeight = m.Height
	}
	limit := width - rightWidth
	for index, object := range objects {
		if object == l.trailing || !object.Visible() {
			continue
		}
		m := object.MinSize()
		minWidth = max(minWidth, m.Width)
		if index == 0 {
			minWidth = max(minWidth, m.Width+rightWidth)
		}
		if x > 0 && x+m.Width > limit {
			y += rowHeight + gap
			x = 0
			rowHeight = 0
			limit = width
		}
		if place {
			object.Move(fyne.NewPos(x, y))
			object.Resize(m)
		}
		x += m.Width + gap
		rowHeight = max(rowHeight, m.Height)
		height = max(height, y+rowHeight)
	}
	if l.trailing != nil && l.trailing.Visible() {
		m := l.trailing.MinSize()
		height = max(height, m.Height)
		if place {
			l.trailing.Move(fyne.NewPos(max(0, width-m.Width), 0))
			l.trailing.Resize(m)
		}
	}
	return fyne.NewSize(minWidth, height)
}

func (l *filterBarLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := l.width
	if width <= 0 {
		width = 100000
	}
	return l.arrange(objects, width, false)
}
func (l *filterBarLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	before := l.MinSize(objects)
	l.width = size.Width
	after := l.arrange(objects, size.Width, true)
	if before.Height != after.Height && l.owner != nil {
		// A width change can change the required height; invalidate the containing
		// canvas so its parent reserves space for the newly wrapped rows.
		if canvas := fyne.CurrentApp().Driver().CanvasForObject(l.owner); canvas != nil {
			canvas.Refresh(l.owner)
		}
	}
}
