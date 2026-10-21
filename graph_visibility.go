package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const (
	graphOutgoing             = "outgoing"
	graphOutgoingLogistics    = "outgoing-logistics"
	graphIncoming             = "incoming"
	graphIncomingLogistics    = "incoming-logistics"
	graphIncomingEWar         = "incoming-ewar"
	graphVisibilityPreference = "graph.visible."
)

var graphVisibilityOptions = []struct{ id, label string }{
	{graphOutgoing, "Outgoing"},
	{graphOutgoingLogistics, "Outgoing logistics"},
	{graphIncoming, "Incoming"},
	{graphIncomingLogistics, "Incoming logistics"},
	{graphIncomingEWar, "Incoming E-War"},
}

func graphVisible(id string) bool {
	return fyne.CurrentApp().Preferences().BoolWithFallback(graphVisibilityPreference+id, id == graphOutgoing || id == graphIncoming)
}

func (ui *analyzerUI) newGraphCard(id, title string, content fyne.CanvasObject) *widget.Card {
	card := widget.NewCard(title, "", content)
	ui.graphCards[id] = card
	return card
}

func (ui *analyzerUI) applyGraphVisibility() {
	if ui.graphsScroll == nil {
		return
	}
	visible := 0
	for id, card := range ui.graphCards {
		if graphVisible(id) {
			card.Show()
			visible++
		} else {
			card.Hide()
		}
	}
	ui.graphsScroll.Content = ui.graphStack
	if visible == 0 {
		ui.graphsScroll.Content = ui.graphsEmpty
	}
	ui.graphsScroll.Refresh()
}

func (ui *analyzerUI) graphVisibilityMenuItems() []*fyne.MenuItem {
	var items []*fyne.MenuItem
	for _, option := range graphVisibilityOptions {
		id := option.id
		item := fyne.NewMenuItem("Show "+option.label, func() {
			fyne.CurrentApp().Preferences().SetBool(graphVisibilityPreference+id, !graphVisible(id))
			ui.applyGraphVisibility()
		})
		item.Checked = graphVisible(id)
		items = append(items, item)
	}
	return items
}

// Fyne dismisses a menu before invoking an item action. Re-show the same
// popup synchronously for visibility toggles, updating its live checkmark
// and restoring the active row so repeated keyboard toggles work too.
func (ui *analyzerUI) keepGraphVisibilityMenuOpen(popup *widget.PopUpMenu, model *fyne.Menu) {
	for _, option := range graphVisibilityOptions {
		id := option.id
		for index, item := range model.Items {
			if item.Label != "Show "+option.label {
				continue
			}
			action := item.Action
			item.Action = func() {
				action()
				item.Checked = graphVisible(id)
				popup.ShowAtPosition(popup.Position())
				for _, row := range model.Items[:index+1] {
					if !row.IsSeparator {
						popup.ActivateNext()
					}
				}
			}
		}
	}
}

func newGraphsEmptyState() fyne.CanvasObject {
	label := widget.NewLabel("All graphs are hidden. Use Options to show a graph.")
	return container.NewCenter(label)
}
