package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type damageFilterPopup struct {
	ui          *analyzerUI
	kind        string
	popup       *damageFilterPopUp
	search      *widget.Entry
	all         *widget.Check
	list        *widget.List
	status      *widget.Label
	done        *widget.Button
	visible     []damageFilterOption
	options     func() []damageFilterOption
	selected    func(string) bool
	setSelected func(string, bool)
}

// Refit whenever Fyne lays out the overlay, including while the window resizes.
type damageFilterPopUp struct {
	widget.PopUp
	anchor    fyne.CanvasObject
	preferred fyne.Size
}

func (popup *damageFilterPopUp) Resize(fyne.Size) {
	const margin float32 = 8
	canvas := popup.Canvas.Size()
	size := popup.preferred.Max(popup.MinSize()).Min(fyne.NewSize(max(0, canvas.Width-2*margin), max(0, canvas.Height-2*margin)))
	anchor := fyne.CurrentApp().Driver().AbsolutePositionForObject(popup.anchor)
	position := anchor.Add(fyne.NewPos(0, popup.anchor.Size().Height))
	if position.Y+size.Height > canvas.Height-margin && anchor.Y-size.Height >= margin {
		position.Y = anchor.Y - size.Height
	}
	position.X = max(margin, min(position.X, canvas.Width-size.Width-margin))
	position.Y = max(margin, min(position.Y, canvas.Height-size.Height-margin))
	popup.PopUp.Resize(size)
	popup.Move(position)
}

type damageFilterRow struct {
	widget.BaseWidget
	check *widget.Check
	label *widget.Label
}

func newDamageFilterRow() *damageFilterRow {
	row := &damageFilterRow{check: widget.NewCheck("", nil), label: widget.NewLabel("Option")}
	row.label.Truncation = fyne.TextTruncateEllipsis
	row.ExtendBaseWidget(row)
	return row
}

func (row *damageFilterRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, row.check, nil, row.label))
}

func (row *damageFilterRow) Tapped(*fyne.PointEvent) { row.check.SetChecked(!row.check.Checked) }

func (ui *analyzerUI) damageFilterBar() fyne.CanvasObject {
	return ui.scopedFilterBar(globalFilterScope, ui.graphOptionsButton)
}
func (ui *analyzerUI) incomingDamageFilterBar() fyne.CanvasObject {
	return container.NewVBox(ui.graphInformationLabels[graphIncoming], ui.scopedFilterBar(graphIncoming, ui.incomingGroupButton))
}

func (ui *analyzerUI) damageFilterCounts(kind string) (int, int) {
	options := ui.filterOptions[kind]
	selected := 0
	for _, option := range options {
		if ui.damageFilterSelected(kind, option.Key) {
			selected++
		}
	}
	return selected, len(options)
}

func (ui *analyzerUI) refreshDamageFilterControls() {
	for scope, buttons := range ui.scopedFilterButtons {
		for kind, button := range buttons {
			button.SetText(ui.scopeFilterLabel(scope, kind))
		}
	}
	if popup := ui.filterPopup; popup != nil && popup.popup.Visible() {
		popup.refresh()
	}
}
func (ui *analyzerUI) openDamageFilter(kind string) {
	scope := globalFilterScope
	if kind == filterNPCNames || kind == filterNPCTypes {
		scope = graphIncoming
	}
	ui.openScopedFilter(scope, kind, ui.filterButtons[kind])
}

func (ui *analyzerUI) openSelectionFilter(kind string, anchor fyne.CanvasObject, options func() []damageFilterOption, selected func(string) bool, setSelected func(string, bool), setAll func(bool)) {
	if previous := ui.filterPopup; previous != nil {
		previous.popup.Hide()
	}
	filter := &damageFilterPopup{ui: ui, kind: kind, search: widget.NewEntry(), status: widget.NewLabel(""), options: options, selected: selected, setSelected: setSelected}
	filter.status.Truncation = fyne.TextTruncateEllipsis
	ui.filterPopup = filter
	filter.search.SetPlaceHolder("Search " + strings.ToLower(kind) + "…")
	filter.search.OnChanged = func(string) { filter.refresh() }
	filter.all = widget.NewCheck("All", setAll)
	filter.list = widget.NewList(func() int { return len(filter.visible) }, func() fyne.CanvasObject { return newDamageFilterRow() }, func(index widget.ListItemID, object fyne.CanvasObject) {
		row := object.(*damageFilterRow)
		option := filter.visible[index]
		row.label.SetText(option.Label)
		row.check.OnChanged = nil
		row.check.SetChecked(filter.selected(option.Key))
		row.check.OnChanged = func(checked bool) { filter.setSelected(option.Key, checked) }
	})
	filter.done = widget.NewButton("Done", func() { filter.popup.Hide() })
	footer := container.NewBorder(nil, nil, nil, filter.done, filter.status)
	header := container.NewVBox(filter.search, filter.all, widget.NewSeparator())
	filter.refresh()
	height := min(float32(440), 132+40*float32(max(1, len(filter.visible))))
	width := float32(380)
	if kind == filterNPCTypes {
		width = 600 // CCP's NPC group names often include faction and encounter.
	}
	filter.popup = &damageFilterPopUp{
		PopUp:  widget.PopUp{Content: container.NewBorder(header, footer, nil, nil, filter.list), Canvas: ui.window.Canvas()},
		anchor: anchor, preferred: fyne.NewSize(width, height),
	}
	filter.popup.ExtendBaseWidget(filter.popup)
	filter.popup.Resize(filter.popup.preferred)
	filter.popup.ShowAtPosition(filter.popup.Position())
	ui.window.Canvas().Focus(filter.search)
}

func (filter *damageFilterPopup) refresh() {
	query := strings.ToLower(strings.TrimSpace(filter.search.Text))
	filter.visible = nil
	options := filter.options()
	selected, total := 0, len(options)
	for _, option := range options {
		if filter.selected(option.Key) {
			selected++
		}
		if strings.Contains(strings.ToLower(option.Label), query) {
			filter.visible = append(filter.visible, option)
		}
	}
	filter.all.Checked = selected == total && total > 0
	filter.all.Partial = selected > 0 && selected < total
	if total == 0 {
		filter.all.Disable()
	} else {
		filter.all.Enable()
	}
	filter.all.Refresh()
	status := fmt.Sprintf("%d of %d selected", selected, total)
	if len(filter.visible) == 0 {
		status = "No matching options · " + status
	}
	filter.status.SetText(status)
	filter.list.Refresh()
}
