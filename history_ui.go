package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

func (ui *analyzerUI) shipLocationHistoryContent() *container.AppTabs {
	logs := ui.logLocations.clone()
	logs.merge(ui.snapshot.locationSessions())
	return container.NewAppTabs(
		container.NewTabItem("Game log locations", logLocationHistoryContent(logs)),
		container.NewTabItem("ESI locations", locationHistoryContent(ui.locationHistory)),
		container.NewTabItem("Ship history", shipHistoryContent(ui.shipHistory)),
	)
}

func (ui *analyzerUI) openShipLocationHistory() {
	viewer := newPopupWindow("Ship & Location History", ui.shipLocationHistoryContent(), ui.window)
	viewer.Resize(fyne.NewSize(1140, 620))
	viewer.Show()
}
