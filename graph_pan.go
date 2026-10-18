package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type graphPanButton struct {
	widget.Button
	forward bool
}

func newGraphPanButton(forward bool, tapped func()) *graphPanButton {
	button := &graphPanButton{forward: forward}
	button.Icon = theme.NavigateBackIcon()
	if forward {
		button.Icon = theme.NavigateNextIcon()
	}
	button.OnTapped = tapped
	button.ExtendBaseWidget(button)
	return button
}

func (button *graphPanButton) MinSize() fyne.Size {
	return fyne.NewSize(theme.Size(theme.SizeNameInlineIcon)+2*theme.Padding(), button.Button.MinSize().Height)
}

func (button *graphPanButton) AccessibilityLabel() string {
	if button.forward {
		return "Move time window forward by 25 percent"
	}
	return "Move time window backward by 25 percent"
}

func (ui *analyzerUI) graphPanContent(chart *damageChart) fyne.CanvasObject {
	back := newGraphPanButton(false, func() { ui.panGraphRange(false) })
	forward := newGraphPanButton(true, func() { ui.panGraphRange(true) })
	return container.NewBorder(nil, nil, back, forward, chart)
}
