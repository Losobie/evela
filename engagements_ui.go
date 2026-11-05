package main

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type engagementsViewer struct {
	content        *fyne.Container
	table          *engagementKeyboardTable
	status         *widget.Label
	selection      *widget.Label
	rows           []engagement
	snapshot       *logSnapshot
	dirty          bool
	loading        bool
	generation     uint64
	cancel         context.CancelFunc
	onOpen         func(engagement)
	dispatch       func(func())
	highlightedRow int
}

type engagementKeyboardTable struct {
	widget.Table
	openHighlighted func()
}

func (table *engagementKeyboardTable) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter {
		table.openHighlighted()
		return
	}
	table.Table.TypedKey(event)
}

func newEngagementsViewer(onOpen func(engagement)) *engagementsViewer {
	return &engagementsViewer{content: container.NewStack(widget.NewLabel("Open Engagements to discover activity in loaded logs.")), dirty: true, onOpen: onOpen, dispatch: fyne.Do}
}

func (viewer *engagementsViewer) initTable() {
	viewer.status = widget.NewLabel("")
	viewer.selection = widget.NewLabel("Double-click an engagement or press Enter on its highlighted row to open Graphs. Existing graph filters still apply.")
	viewer.selection.Wrapping = fyne.TextWrapWord
	keyboardTable := &engagementKeyboardTable{openHighlighted: func() {
		if row := viewer.highlightedRow - 1; row >= 0 && row < len(viewer.rows) {
			viewer.onOpen(viewer.rows[row])
		}
	}}
	keyboardTable.ExtendBaseWidget(keyboardTable)
	viewer.table = keyboardTable
	viewer.table.Length = func() (int, int) { return len(viewer.rows) + 1, 3 }
	viewer.table.CreateCell = func() fyne.CanvasObject { return newEngagementCell() }
	viewer.table.UpdateCell = func(id widget.TableCellID, object fyne.CanvasObject) {
		cell := object.(*engagementCell)
		cell.label.TextStyle.Bold = id.Row == 0
		cell.onOpen, cell.onSelect = nil, nil
		if id.Row == 0 {
			cell.label.SetText([]string{"Timestamp · EVE time ↓", "Duration", "Summary"}[id.Col])
			return
		}
		row := viewer.rows[id.Row-1]
		cell.label.SetText([]string{row.Start.Format("2006-01-02 15:04:05"), engagementDuration(row.End.Sub(row.Start)), row.Summary}[id.Col])
		cell.onOpen = func() { viewer.onOpen(row) }
		cell.onSelect = func() {
			viewer.table.Select(id)
			fyne.CurrentApp().Driver().CanvasForObject(viewer.table).Focus(keyboardTable)
		}
	}
	showSelection := func(id widget.TableCellID) {
		viewer.highlightedRow = id.Row
		if id.Row > 0 && id.Row <= len(viewer.rows) {
			viewer.selection.SetText(viewer.rows[id.Row-1].Summary + "\nDouble-click or press Enter to open in Graphs · Existing graph filters still apply.")
		}
	}
	viewer.table.OnSelected, viewer.table.OnHighlighted = showSelection, showSelection
	viewer.table.StickyRowCount = 1
	table := newFillingTable(&viewer.table.Table, 220, 100)
	table.Objects[0] = viewer.table
	note := widget.NewLabel("All loaded logs · 10-minute inactivity gap · Damage, E-War, mining and nonzero repairs · Independent of graph range and filters")
	note.Wrapping = fyne.TextWrapWord
	viewer.content.Objects = []fyne.CanvasObject{container.NewBorder(container.NewVBox(note, viewer.status), viewer.selection, nil, nil, table)}
	viewer.content.Refresh()
}

func engagementDuration(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %dm", seconds/3600, seconds%3600/60)
}

func (viewer *engagementsViewer) cancelBuild() {
	if viewer == nil || viewer.cancel == nil {
		return
	}
	viewer.cancel()
	viewer.cancel = nil
	viewer.generation++
	viewer.loading = false
	viewer.dirty = true
}

func (viewer *engagementsViewer) invalidate() {
	if viewer == nil {
		return
	}
	viewer.cancelBuild()
	viewer.dirty = true
	viewer.snapshot = nil
}

func (ui *analyzerUI) refreshEngagements() {
	viewer := ui.engagements
	if viewer == nil {
		return
	}
	if viewer.snapshot != ui.snapshot {
		viewer.invalidate()
	}
	if ui.tabs == nil || ui.tabs.Selected().Content != viewer.content {
		return
	}
	if viewer.loading || !viewer.dirty {
		return
	}
	if viewer.table == nil {
		viewer.initTable()
	}
	viewer.status.SetText("Finding engagements…")
	viewer.snapshot = ui.snapshot
	viewer.loading = true
	ctx, cancel := context.WithCancel(context.Background())
	viewer.cancel = cancel
	viewer.generation++
	generation := viewer.generation
	snapshot, locations, ships := ui.snapshot, ui.resolvedLocations(), ui.shipHistory.clone()
	dispatch := viewer.dispatch
	go func() {
		rows, err := buildEngagements(ctx, snapshot, locations, ships)
		if err != nil {
			return // Cancelled by navigation, refresh or closing the application.
		}
		dispatch(func() {
			if generation != viewer.generation {
				return
			}
			cancel()
			viewer.cancel = nil
			viewer.rows = rows
			viewer.status.SetText(fmt.Sprintf("%d engagements · Most recent first", len(rows)))
			if len(rows) == 0 {
				viewer.status.SetText("No combat, E-War, mining or nonzero repair activity in loaded logs.")
			}
			viewer.table.UnselectAll()
			viewer.highlightedRow = 0
			viewer.selection.SetText("Double-click an engagement or press Enter on its highlighted row to open Graphs. Existing graph filters still apply.")
			viewer.table.Refresh()
			viewer.loading, viewer.dirty = false, false
		})
	}()
}

func (ui *analyzerUI) openEngagement(row engagement) {
	if !row.End.After(row.Start) {
		return
	}
	// Graph bounds use whole seconds. Round padding outward so both sides get
	// context, even for engagements lasting only a few seconds.
	padding := (row.End.Sub(row.Start)/10 + time.Second - 1).Truncate(time.Second)
	ui.applyGraphRange(row.Start.Add(-padding), row.End.Add(padding))
	ui.tabs.SelectIndex(0)
}

// Each virtualized cell handles real desktop double-clicks. Its callbacks are
// rebound on reuse, so scrolling or refreshing cannot open an obsolete row.
type engagementCell struct {
	widget.BaseWidget
	label            *widget.Label
	onOpen, onSelect func()
}

func newEngagementCell() *engagementCell {
	cell := &engagementCell{label: widget.NewLabel("Engagement")}
	cell.label.Truncation = fyne.TextTruncateEllipsis
	cell.ExtendBaseWidget(cell)
	return cell
}

func (cell *engagementCell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(cell.label)
}

func (cell *engagementCell) Tapped(*fyne.PointEvent) {
	if cell.onSelect != nil {
		cell.onSelect()
	}
}

func (cell *engagementCell) DoubleTapped(*fyne.PointEvent) {
	if cell.onOpen != nil {
		cell.onOpen()
	}
}
