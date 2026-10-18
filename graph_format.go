package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) initGraphFormat() {
	ui.graphFormatLabel = widget.NewLabel("")
	ui.graphFormatLabel.Wrapping = fyne.TextWrapWord
	ui.graphFormatLabel.Truncation = fyne.TextTruncateEllipsis
	ui.graphFormatButton = widget.NewButtonWithIcon("Format options", theme.MenuDropDownIcon(), func() {
		menu := widget.NewPopUpMenu(ui.graphFormatMenu(), ui.window.Canvas())
		menu.ShowAtRelativePosition(fyne.NewPos(ui.graphFormatButton.Size().Width-menu.MinSize().Width,
			ui.graphFormatButton.Size().Height), ui.graphFormatButton)
	})
	ui.updateGraphFormatLabel()
}

func (ui *analyzerUI) graphFormatMenu() *fyne.Menu {
	linear := fyne.NewMenuItem("Linear", func() { ui.graphLogarithmic = false; ui.renderSelection() })
	linear.Checked = !ui.graphLogarithmic
	logarithmic := fyne.NewMenuItem("Logarithmic (DPS + 1)", func() { ui.graphLogarithmic = true; ui.renderSelection() })
	logarithmic.Checked = ui.graphLogarithmic
	characters := fyne.NewMenuItem("Per character", func() { ui.groupSelect.SetSelected("Characters") })
	characters.Checked = !ui.shipGrouping()
	ships := fyne.NewMenuItem("By ship type (ship history)", func() { ui.groupSelect.SetSelected("Ship history") })
	ships.Checked = ui.shipGrouping() && !ui.shipClassGrouping()
	classes := fyne.NewMenuItem("By ship class (ship history)", func() { ui.groupSelect.SetSelected("Ship class history") })
	classes.Checked = ui.shipClassGrouping()
	average := fyne.NewMenuItem("10-second DPS average", func() {
		ui.graphAverage10s = !ui.graphAverage10s
		ui.renderSelection()
		ui.rebuildCharacters()
	})
	average.Checked = ui.graphAverage10s
	return fyne.NewMenu("Format options", linear, logarithmic, fyne.NewMenuItemSeparator(), characters, ships, classes, fyne.NewMenuItemSeparator(), average)
}

func (ui *analyzerUI) updateGraphFormatLabel() {
	if ui.graphFormatLabel == nil {
		return
	}
	scale, grouping := "Linear", "Per character"
	if ui.graphLogarithmic {
		scale = "Logarithmic (DPS + 1)"
	}
	if ui.shipGrouping() {
		grouping = "By ship type · history at event time"
	}
	if ui.shipClassGrouping() {
		grouping = "By ship class · history at event time"
	}
	text := scale + " · " + grouping
	if ui.graphAverage10s {
		text += " · 10-second average"
	}
	ui.graphFormatLabel.SetText(text)
}
