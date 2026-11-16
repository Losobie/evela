package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func selectionFixture(t *testing.T) (*cellSelectionTable, fyne.Window) {
	app := test.NewTempApp(t)
	window := app.NewWindow("Multiple cells")
	t.Cleanup(window.Close)
	table := newCellSelectionTable(func() (int, int) { return 20, 4 }, func() fyne.CanvasObject { return widget.NewLabel("") }, func(id widget.TableCellID, object fyne.CanvasObject) {
		label := object.(*widget.Label)
		label.Truncation = fyne.TextTruncateEllipsis
		label.SetText(fmt.Sprintf("R%dC%d", id.Row, id.Col))
	}, func(object fyne.CanvasObject) string { return object.(*widget.Label).Text })
	table.StickyRowCount = 1
	table.StickyColumnCount = 1
	for col := 0; col < 4; col++ {
		table.SetColumnWidth(col, 100)
	}
	window.SetContent(table)
	window.Resize(fyne.NewSize(450, 300))
	window.Show()
	return table, window
}

func renderedSelectionCell(t *testing.T, table *cellSelectionTable, id widget.TableCellID) *selectionTableCell {
	t.Helper()
	var found *selectionTableCell
	var visit func(fyne.CanvasObject)
	visit = func(object fyne.CanvasObject) {
		if !object.Visible() {
			return
		}
		if cell, ok := object.(*selectionTableCell); ok {
			if cell.id == id {
				found = cell
			}
			return
		}
		switch object := object.(type) {
		case *fyne.Container:
			for _, child := range object.Objects {
				visit(child)
			}
		case fyne.Widget:
			for _, child := range test.WidgetRenderer(object).Objects() {
				visit(child)
			}
		}
	}
	visit(table)
	if found == nil {
		t.Fatalf("visible cell %+v not found", id)
	}
	return found
}

func selectionCopy(table *cellSelectionTable) string {
	table.TypedShortcut(&fyne.ShortcutCopy{})
	return fyne.CurrentApp().Clipboard().Content()
}

func selectionTableScroll(t *testing.T, table *cellSelectionTable) *container.Scroll {
	t.Helper()
	for _, object := range test.WidgetRenderer(table).Objects() {
		if scroll, ok := object.(*container.Scroll); ok {
			return scroll
		}
	}
	t.Fatal("table scroll viewport missing")
	return nil
}

func TestCellMouseMovementPreservesSelection(t *testing.T) {
	t.Run("hover does not select", func(t *testing.T) {
		table, window := selectionFixture(t)
		cell := renderedSelectionCell(t, table, widget.TableCellID{Row: 2, Col: 1})
		position := fyne.CurrentApp().Driver().AbsolutePositionForObject(cell).Add(fyne.NewPos(10, 10))
		test.MoveMouse(window.Canvas(), position)
		if len(table.selected) != 0 || table.hasAnchor {
			t.Fatal("hover selected a cell without a mouse press")
		}
	})
	t.Run("hover during and after drag preserves range", func(t *testing.T) {
		table, window := selectionFixture(t)
		first := renderedSelectionCell(t, table, widget.TableCellID{Row: 1, Col: 1})
		first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
		start := fyne.CurrentApp().Driver().AbsolutePositionForObject(first)
		for _, id := range []widget.TableCellID{{Row: 2, Col: 1}, {Row: 3, Col: 2}} {
			cell := renderedSelectionCell(t, table, id)
			position := fyne.CurrentApp().Driver().AbsolutePositionForObject(cell).Add(fyne.NewPos(10, 10))
			// The desktop driver sends hover before the captured drag event.
			test.MoveMouse(window.Canvas(), position)
			first.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{AbsolutePosition: position, Position: position.Subtract(start)}})
			if table.anchor != (widget.TableCellID{Row: 1, Col: 1}) || len(table.selected) != id.Row*id.Col {
				t.Fatalf("mouse movement reset the drag range: anchor=%v selected=%v", table.anchor, table.selected)
			}
		}
		first.DragEnd()
		cell := renderedSelectionCell(t, table, widget.TableCellID{Row: 4, Col: 3})
		test.MoveMouse(window.Canvas(), fyne.CurrentApp().Driver().AbsolutePositionForObject(cell).Add(fyne.NewPos(10, 10)))
		if got := selectionCopy(table); got != "R1C1\tR1C2\nR2C1\tR2C2\nR3C1\tR3C2" {
			t.Fatalf("hover after release changed copied range: %q", got)
		}
	})
}

func TestCellMouseDownIsImmediateAndDelayedTapDoesNotCollapseRange(t *testing.T) {
	table, window := selectionFixture(t)
	first := renderedSelectionCell(t, table, widget.TableCellID{Row: 1, Col: 1})
	opened := 0
	table.onActivate = func(widget.TableCellID) { opened++ }
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if !table.selected[first.id] || window.Canvas().Focused() != table || opened != 0 {
		t.Fatal("MouseDown did not immediately select/focus without activating")
	}
	target := renderedSelectionCell(t, table, widget.TableCellID{Row: 3, Col: 2})
	start := fyne.CurrentApp().Driver().AbsolutePositionForObject(first)
	end := fyne.CurrentApp().Driver().AbsolutePositionForObject(target).Add(fyne.NewPos(10, 10))
	first.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: end.Subtract(start)}})
	first.DragEnd()
	first.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	first.Tapped(&fyne.PointEvent{})
	if len(table.selected) != 6 || selectionCopy(table) != "R1C1\tR1C2\nR2C1\tR2C2\nR3C1\tR3C2" || opened != 0 {
		t.Fatalf("drag/release lost rectangle: %v", table.selected)
	}
	target.DoubleTapped(&fyne.PointEvent{})
	if opened != 1 {
		t.Fatal("double-click did not activate separately")
	}
}

func TestCellCtrlAndShiftSelectionAndSparseCopy(t *testing.T) {
	table, _ := selectionFixture(t)
	press := func(row, col int, mods fyne.KeyModifier) {
		cell := renderedSelectionCell(t, table, widget.TableCellID{Row: row, Col: col})
		cell.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: mods})
		cell.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
		cell.Tapped(&fyne.PointEvent{})
	}
	press(1, 0, 0)
	press(1, 2, fyne.KeyModifierControl)
	if got := selectionCopy(table); got != "R1C0\t\tR1C2" {
		t.Fatalf("sparse copy = %q", got)
	}
	press(1, 0, fyne.KeyModifierControl)
	if len(table.selected) != 1 || !table.selected[widget.TableCellID{Row: 1, Col: 2}] {
		t.Fatal("Ctrl-click did not deselect the existing cell")
	}
	press(3, 3, fyne.KeyModifierShift)
	if len(table.selected) != 12 {
		t.Fatalf("Shift-click did not extend the anchor rectangle: %v", table.selected)
	}
	press(5, 0, fyne.KeyModifierControl|fyne.KeyModifierShift)
	if !table.selected[widget.TableCellID{Row: 5, Col: 0}] || !table.selected[widget.TableCellID{Row: 1, Col: 3}] {
		t.Fatal("Ctrl+Shift did not add a range")
	}
	press(2, 1, 0)
	if len(table.selected) != 1 {
		t.Fatal("plain click did not replace multi-selection")
	}
	table.Refresh()
	fyne.CurrentApp().Clipboard().SetContent("unchanged")
	table.TypedShortcut(&fyne.ShortcutCopy{})
	if len(table.selected) != 0 || fyne.CurrentApp().Clipboard().Content() != "unchanged" {
		t.Fatal("data refresh retained stale selection")
	}
}

func TestCellDragAfterScrollAndStickyColumn(t *testing.T) {
	table, _ := selectionFixture(t)
	table.ScrollTo(widget.TableCellID{Row: 12, Col: 2})
	first := renderedSelectionCell(t, table, widget.TableCellID{Row: 11, Col: 0})
	last := renderedSelectionCell(t, table, widget.TableCellID{Row: 12, Col: 2})
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	position := fyne.CurrentApp().Driver().AbsolutePositionForObject(last).Add(fyne.NewPos(5, 5)).Subtract(fyne.CurrentApp().Driver().AbsolutePositionForObject(first))
	first.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: position}})
	first.DragEnd()
	if len(table.selected) != 6 || !strings.Contains(selectionCopy(table), "R12C2") {
		t.Fatalf("scrolled sticky-column drag selected wrong cells: %v", table.selected)
	}
}

func TestEventsSelectionDoesNotActivateAndCopiesFullMessages(t *testing.T) {
	ui, _ := newEventsTestUI(t)
	table := ui.events.table
	cell := renderedSelectionCell(t, table, widget.TableCellID{Row: 1, Col: 2})
	cell.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if ui.window.Canvas().Overlays().Top() != nil || selectionCopy(table) != ui.events.rows[0].Message {
		t.Fatal("MouseDown opened details or copied an incorrect message")
	}
	second := renderedSelectionCell(t, table, widget.TableCellID{Row: 2, Col: 2})
	second.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierShift})
	if len(table.selected) != 2 {
		t.Fatal("Events did not share range selection")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_MULTISELECT_CAPTURE"))
	second.DoubleTapped(&fyne.PointEvent{})
	if ui.window.Canvas().Overlays().Top() != nil {
		t.Fatal("Shift-double-click activated instead of selecting")
	}
	second.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	second.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	second.DoubleTapped(&fyne.PointEvent{})
	if ui.window.Canvas().Overlays().Top() == nil {
		t.Fatal("double-click did not open event details")
	}
	ui.window.Canvas().Overlays().Remove(ui.window.Canvas().Overlays().Top())
	ui.events.search.SetText("hello")
	if len(table.selected) != 0 {
		t.Fatal("filtering retained a stale event selection")
	}
}

func TestTotalsSelectionAndKeyboardCopy(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	ui.tabs.SelectIndex(1)
	table := ui.totals
	cell := renderedSelectionCell(t, table, widget.TableCellID{Row: 1, Col: 0})
	cell.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if selectionCopy(table) != ui.tableSeries[0].Name {
		t.Fatal("Totals copied wrong value")
	}
	cell = renderedSelectionCell(t, table, widget.TableCellID{Row: 2, Col: 1})
	cell.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierShift})
	if len(table.selected) != 4 {
		t.Fatal("Totals did not select a rectangle")
	}
	table.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if len(table.selected) != 1 || table.active.Col != 2 {
		t.Fatal("keyboard movement did not select the next cell")
	}
	direction := ui.totalsSortDescending
	header := renderedSelectionCell(t, table, widget.TableCellID{Row: 0, Col: 0})
	header.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if ui.totalsSortDescending == direction {
		t.Fatal("heading did not sort on mouse-down")
	}
	if len(table.selected) != 0 {
		t.Fatal("sorting retained stale selection")
	}
}

func TestEngagementCellRangesSelectBeforeMouseUpWithoutNavigation(t *testing.T) {
	app := test.NewTempApp(t)
	window := app.NewWindow("Engagement ranges")
	t.Cleanup(window.Close)
	opened := 0
	viewer := newEngagementsViewer(func(engagement) { opened++ })
	viewer.rows = []engagement{{Summary: "First"}, {Summary: "Second"}, {Summary: "Third"}}
	viewer.initTable()
	window.SetContent(viewer.content)
	window.Resize(fyne.NewSize(900, 400))
	window.Show()
	first := renderedSelectionCell(t, viewer.table, widget.TableCellID{Row: 1, Col: 0})
	first.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	if len(viewer.table.selected) != 1 || opened != 0 {
		t.Fatal("engagement did not select immediately without navigating")
	}
	last := renderedSelectionCell(t, viewer.table, widget.TableCellID{Row: 3, Col: 2})
	last.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary, Modifier: fyne.KeyModifierShift})
	if len(viewer.table.selected) != 9 || opened != 0 || !strings.Contains(selectionCopy(viewer.table), "Third") {
		t.Fatal("engagement Shift selection did not select/copy the range")
	}
	last.DoubleTapped(&fyne.PointEvent{})
	if opened != 0 {
		t.Fatal("modified double-click navigated while selecting")
	}
	last.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	last.MouseUp(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})
	last.DoubleTapped(&fyne.PointEvent{})
	if opened != 1 {
		t.Fatal("plain double-click did not open the engagement")
	}
}
