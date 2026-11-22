package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type engagementsViewer struct {
	content        *fyne.Container
	table          *cellSelectionTable
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

func newEngagementsViewer(onOpen func(engagement)) *engagementsViewer {
	return &engagementsViewer{content: container.NewStack(widget.NewLabel("Open Engagements to discover activity in loaded logs.")), dirty: true, onOpen: onOpen, dispatch: fyne.Do}
}

func (viewer *engagementsViewer) initTable() {
	viewer.status = widget.NewLabel("")
	viewer.selection = widget.NewLabel("Double-click an engagement or press Enter on its highlighted row to open Graphs. Existing graph filters still apply.")
	viewer.selection.Wrapping = fyne.TextWrapWord
	viewer.table = newCellSelectionTable(nil, nil, nil, func(object fyne.CanvasObject) string { return object.(*engagementCell).label.Text })
	viewer.table.Length = func() (int, int) { return len(viewer.rows) + 1, 3 }
	viewer.table.createCell = func() fyne.CanvasObject { return newEngagementCell() }
	viewer.table.updateCell = func(id widget.TableCellID, object fyne.CanvasObject) {
		cell := object.(*engagementCell)
		cell.label.TextStyle.Bold = id.Row == 0
		if id.Row == 0 {
			cell.label.SetText([]string{"Timestamp · EVE time ↓", "Duration", "Summary"}[id.Col])
			return
		}
		row := viewer.rows[id.Row-1]
		cell.label.SetText([]string{row.Start.Format("2006-01-02 15:04:05"), engagementDuration(row.End.Sub(row.Start)), row.Summary}[id.Col])
	}
	showSelection := func(id widget.TableCellID) {
		viewer.highlightedRow = id.Row
		if id.Row > 0 && id.Row <= len(viewer.rows) {
			viewer.selection.SetText(viewer.rows[id.Row-1].Summary + "\nDouble-click or press Enter to open in Graphs · Existing graph filters still apply.")
		}
	}
	viewer.table.selectionChanged = showSelection
	viewer.table.onActivate = func(id widget.TableCellID) { viewer.onOpen(viewer.rows[id.Row-1]) }
	viewer.table.StickyRowCount = 1
	table := newFillingTable(viewer.table, 220, 100)
	viewer.content.Objects = []fyne.CanvasObject{container.NewBorder(viewer.status, viewer.selection, nil, nil, table)}
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
	if ui.foregroundReady != nil {
		return
	}
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
	start, end := engagementGraphRange(row, ui.snapshot)
	ui.applyGraphRange(start, end)
	ui.tabs.SelectIndex(0)
}

func engagementGraphRange(row engagement, snapshot *logSnapshot) (time.Time, time.Time) {
	// Graph bounds use whole seconds. Round padding outward so both sides get
	// context, even for engagements lasting only a few seconds.
	padding := (row.End.Sub(row.Start)/10 + time.Second - 1).Truncate(time.Second)
	start, end := row.Start.Add(-padding), row.End.Add(padding)
	const nearby = 30 * time.Second
	if padding >= nearby+time.Second {
		return start, end
	}
	// Notifications and travel can explain the activity without defining the
	// engagement. Look near its original edges so context cannot chain outward.
	first, last := row.Start.Add(-nearby).Unix(), row.End.Add(nearby).Unix()
	startSecond, endSecond := start.Unix(), end.Unix()
	for _, log := range snapshot.Logs {
		seconds := log.EntrySeconds
		left, _ := slices.BinarySearch(seconds, first)
		right, _ := slices.BinarySearch(seconds, last)
		if left < right {
			startSecond = min(startSecond, seconds[left]-1)
			endSecond = max(endSecond, seconds[right-1]+2) // End is exclusive.
		}
	}
	return time.Unix(startSecond, 0).UTC(), time.Unix(endSecond, 0).UTC()
}

// The shared selection surface owns input; cell contents only render text.
type engagementCell struct {
	widget.BaseWidget
	label *widget.Label
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
