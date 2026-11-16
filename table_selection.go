package main

import (
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Selection belongs to data coordinates, not virtualized cell widgets. Model
// refreshes clear it so sorting/filtering cannot copy a different row by mistake.
type cellSelectionTable struct {
	widget.Table
	selected                      map[widget.TableCellID]bool
	anchor, active                widget.TableCellID
	hasAnchor, dragging, additive bool
	dragBase                      map[widget.TableCellID]bool
	locating                      bool
	pressed                       bool
	located                       *widget.TableCellID
	createCell                    func() fyne.CanvasObject
	updateCell                    func(widget.TableCellID, fyne.CanvasObject)
	cellText                      func(fyne.CanvasObject) string
	onActivate                    func(widget.TableCellID)
	selectionChanged              func(widget.TableCellID)
}

func newCellSelectionTable(length func() (int, int), create func() fyne.CanvasObject, update func(widget.TableCellID, fyne.CanvasObject), text func(fyne.CanvasObject) string) *cellSelectionTable {
	t := &cellSelectionTable{createCell: create, updateCell: update, cellText: text, selected: map[widget.TableCellID]bool{}}
	t.Length = length
	t.CreateCell = func() fyne.CanvasObject {
		cell := &selectionTableCell{table: t, content: t.createCell(), background: canvas.NewRectangle(color.Transparent)}
		cell.ExtendBaseWidget(cell)
		return cell
	}
	t.UpdateCell = func(id widget.TableCellID, object fyne.CanvasObject) {
		if cell, ok := object.(*selectionTableCell); ok {
			cell.id = id
			t.updateCell(id, cell.content)
			cell.Refresh()
		} else {
			t.updateCell(id, object)
		}
	}
	t.OnSelected = func(id widget.TableCellID) {
		if t.locating {
			t.located = &id
			return
		}
		t.press(id, 0)
	}
	// Fyne's OnHighlighted reports pointer hover as well as keyboard movement.
	// Selection is handled explicitly by mouse presses, drags and TypedKey;
	// hover must never replace a selection or move the drag anchor.
	t.ExtendBaseWidget(t)
	return t
}

func (t *cellSelectionTable) CreateRenderer() fyne.WidgetRenderer {
	base := t.Table.CreateRenderer()
	objects := base.Objects()
	// Fyne 2.8 places a draggable separator/marker clip after the frozen
	// cells. It captures their drags and forwards them to the base Table's
	// resize handler. Put it underneath the cell layers so frozen cells own
	// their input, just like the cells in the scrolling body.
	return &selectionTableRenderer{WidgetRenderer: base, objects: []fyne.CanvasObject{
		objects[3], objects[0], objects[1], objects[2], objects[4],
	}}
}

type selectionTableRenderer struct {
	fyne.WidgetRenderer
	objects []fyne.CanvasObject
}

func (r *selectionTableRenderer) Objects() []fyne.CanvasObject { return r.objects }

func cloneCells(input map[widget.TableCellID]bool) map[widget.TableCellID]bool {
	result := make(map[widget.TableCellID]bool, len(input))
	for id, checked := range input {
		result[id] = checked
	}
	return result
}

func (t *cellSelectionTable) valid(id widget.TableCellID) bool {
	rows, cols := t.Length()
	return id.Row > 0 && id.Row < rows && id.Col >= 0 && id.Col < cols
}

func (t *cellSelectionTable) clearSelection() {
	t.selected = map[widget.TableCellID]bool{}
	t.hasAnchor = false
	t.active = widget.TableCellID{}
	t.dragging = false
	t.dragBase = nil
	t.Table.UnselectAll()
}

func (t *cellSelectionTable) Refresh() {
	t.clearSelection()
	t.Table.Refresh()
}

func (t *cellSelectionTable) choose(id widget.TableCellID, mods fyne.KeyModifier) {
	if !t.valid(id) {
		return
	}
	additive := mods&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0
	if mods&fyne.KeyModifierShift != 0 && t.hasAnchor {
		if !additive {
			t.selected = map[widget.TableCellID]bool{}
		}
		t.rectangle(t.anchor, id)
	} else {
		if !additive {
			t.selected = map[widget.TableCellID]bool{}
		}
		if t.selected[id] {
			delete(t.selected, id)
		} else {
			t.selected[id] = true
		}
		t.anchor = id
		t.hasAnchor = true
	}
	t.active = id
	t.Table.Highlight(id)
	if t.selectionChanged != nil {
		t.selectionChanged(id)
	}
	t.Table.Refresh()
}

func (t *cellSelectionTable) rectangle(a, b widget.TableCellID) {
	for r := min(a.Row, b.Row); r <= max(a.Row, b.Row); r++ {
		for c := min(a.Col, b.Col); c <= max(a.Col, b.Col); c++ {
			t.selected[widget.TableCellID{Row: r, Col: c}] = true
		}
	}
}

func (t *cellSelectionTable) press(id widget.TableCellID, mods fyne.KeyModifier) {
	if id.Row == 0 {
		t.clearSelection()
		if t.selectionChanged != nil {
			t.selectionChanged(id)
		}
		return
	}
	if !t.valid(id) {
		return
	}
	t.additive = mods&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0
	t.dragBase = cloneCells(t.selected)
	if mods&fyne.KeyModifierShift == 0 || !t.hasAnchor {
		t.anchor = id
		t.hasAnchor = true
	}
	t.choose(id, mods)
	t.dragging = true
	if c := fyne.CurrentApp().Driver().CanvasForObject(t); c != nil {
		c.Focus(t)
	}
}

func (t *cellSelectionTable) drag(id widget.TableCellID) {
	if !t.dragging || !t.valid(id) || id == t.active {
		return
	}
	if t.additive {
		t.selected = cloneCells(t.dragBase)
	} else {
		t.selected = map[widget.TableCellID]bool{}
	}
	t.rectangle(t.anchor, id)
	t.active = id
	if t.selectionChanged != nil {
		t.selectionChanged(id)
	}
	t.Table.Highlight(id)
	t.Table.Refresh()
}

func (t *cellSelectionTable) dragAt(position fyne.Position) {
	if !t.dragging {
		return
	}
	// Let Fyne resolve scrolling, sticky columns and row/column sizes. Suppress
	// normal selection/actions while using its public hit-testing path.
	t.locating = true
	t.located = nil
	t.Table.UnselectAll()
	t.Table.Tapped(&fyne.PointEvent{Position: position})
	t.Table.UnselectAll()
	t.locating = false
	if t.located != nil {
		t.drag(*t.located)
	}
}

func (t *cellSelectionTable) MouseDown(event *desktop.MouseEvent) {
	if event.Button != desktop.MouseButtonPrimary {
		return
	}
	t.pressed = true
	t.locating = true
	t.located = nil
	t.Table.UnselectAll()
	t.Table.Tapped(&event.PointEvent)
	t.Table.UnselectAll()
	t.locating = false
	if t.located != nil {
		t.press(*t.located, event.Modifier)
	}
}
func (t *cellSelectionTable) MouseUp(*desktop.MouseEvent) { t.dragging = false }
func (t *cellSelectionTable) Tapped(event *fyne.PointEvent) {
	// A native mouse press already selected synchronously. Keep a fallback
	// for touch and toolkit-generated taps which have no MouseDown event.
	if !t.pressed {
		t.MouseDown(&desktop.MouseEvent{PointEvent: *event, Button: desktop.MouseButtonPrimary})
		t.dragging = false
	}
	t.pressed = false
}
func (t *cellSelectionTable) Dragged(event *fyne.DragEvent) { t.dragAt(event.Position) }
func (t *cellSelectionTable) DragEnd()                      { t.dragging = false }
func (t *cellSelectionTable) FocusGained()                  {} // Draw our multi-selection, not Fyne's single-cell marker.
func (t *cellSelectionTable) FocusLost()                    { t.Table.Refresh() }

func (t *cellSelectionTable) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter {
		if t.selected[t.active] && t.valid(t.active) && t.onActivate != nil {
			t.onActivate(t.active)
		}
		return
	}
	if event.Name == fyne.KeyEscape {
		t.clearSelection()
		t.Table.Refresh()
		return
	}
	next := t.active
	if !t.hasAnchor {
		next = widget.TableCellID{Row: 1}
	}
	switch event.Name {
	case fyne.KeyLeft:
		next.Col--
	case fyne.KeyRight:
		next.Col++
	case fyne.KeyUp:
		next.Row--
	case fyne.KeyDown:
		next.Row++
	default:
		return
	}
	if !t.hasAnchor {
		next = widget.TableCellID{Row: 1}
	}
	rows, cols := t.Length()
	next.Row = max(1, min(next.Row, rows-1))
	next.Col = max(0, min(next.Col, cols-1))
	mods := fyne.KeyModifier(0)
	if driver, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		mods = driver.CurrentKeyModifiers() & fyne.KeyModifierShift
	}
	t.choose(next, mods)
	t.Table.ScrollTo(next)
}

func (t *cellSelectionTable) TypedShortcut(shortcut fyne.Shortcut) {
	copy, ok := shortcut.(*fyne.ShortcutCopy)
	if !ok || len(t.selected) == 0 {
		return
	}
	rows := map[int]bool{}
	minCol, maxCol := int(^uint(0)>>1), 0
	for id := range t.selected {
		if t.valid(id) {
			rows[id.Row] = true
			minCol = min(minCol, id.Col)
			maxCol = max(maxCol, id.Col)
		}
	}
	ordered := make([]int, 0, len(rows))
	for row := range rows {
		ordered = append(ordered, row)
	}
	sort.Ints(ordered)
	if len(ordered) == 0 {
		return
	}
	template := t.createCell()
	lines := make([]string, 0, len(ordered))
	for _, row := range ordered {
		values := make([]string, maxCol-minCol+1)
		for col := minCol; col <= maxCol; col++ {
			id := widget.TableCellID{Row: row, Col: col}
			if t.selected[id] {
				t.updateCell(id, template)
				values[col-minCol] = t.cellText(template)
			}
		}
		lines = append(lines, strings.Join(values, "\t"))
	}
	clipboard := copy.Clipboard
	if clipboard == nil {
		clipboard = fyne.CurrentApp().Clipboard()
	}
	clipboard.SetContent(strings.Join(lines, "\n"))
}

type selectionTableCell struct {
	widget.BaseWidget
	table      *cellSelectionTable
	id         widget.TableCellID
	content    fyne.CanvasObject
	background *canvas.Rectangle
	pressed    bool
	dragged    bool
	modified   bool
}

func (cell *selectionTableCell) CreateRenderer() fyne.WidgetRenderer {
	return &selectionCellRenderer{WidgetRenderer: widget.NewSimpleRenderer(container.NewStack(cell.background, cell.content)), cell: cell}
}
func (cell *selectionTableCell) MouseDown(event *desktop.MouseEvent) {
	if event.Button == desktop.MouseButtonPrimary {
		cell.pressed = true
		cell.dragged = false
		cell.modified = event.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierShift|fyne.KeyModifierSuper) != 0
		cell.table.press(cell.id, event.Modifier)
	}
}
func (cell *selectionTableCell) MouseUp(*desktop.MouseEvent) { cell.table.dragging = false }
func (cell *selectionTableCell) Tapped(*fyne.PointEvent) {
	if !cell.pressed {
		cell.table.press(cell.id, 0)
		cell.table.dragging = false
	}
	cell.pressed = false
}
func (cell *selectionTableCell) DoubleTapped(*fyne.PointEvent) {
	if !cell.modified && !cell.dragged && cell.table.valid(cell.id) && cell.table.onActivate != nil {
		cell.table.onActivate(cell.id)
	}
}
func (cell *selectionTableCell) Dragged(event *fyne.DragEvent) {
	cell.dragged = true
	position := fyne.CurrentApp().Driver().AbsolutePositionForObject(cell).Add(event.Position).Subtract(fyne.CurrentApp().Driver().AbsolutePositionForObject(cell.table))
	cell.table.dragAt(position)
}
func (cell *selectionTableCell) DragEnd() { cell.table.dragging = false }

type selectionCellRenderer struct {
	fyne.WidgetRenderer
	cell *selectionTableCell
}

func (r *selectionCellRenderer) Refresh() {
	r.cell.background.FillColor = color.Transparent
	if r.cell.table.selected[r.cell.id] {
		r.cell.background.FillColor = r.cell.Theme().Color(theme.ColorNameSelection, fyne.CurrentApp().Settings().ThemeVariant())
	}
	r.cell.background.Refresh()
	r.WidgetRenderer.Refresh()
}
