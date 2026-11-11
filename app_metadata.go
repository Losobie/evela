package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

const applicationID = "io.evela.damage-analyzer"

func newEvelaApp() fyne.App {
	// Declare before constructing the driver: Fyne caches migration state on
	// its first threading check. Runtime metadata also covers plain go builds,
	// which do not embed FyneApp.toml through the Fyne packaging tool.
	app.SetMetadata(fyne.AppMetadata{
		ID:         applicationID,
		Name:       "Evela",
		Icon:       fyne.NewStaticResource("icon.svg", applicationIcon),
		Migrations: map[string]bool{"fyneDo": true},
	})
	return app.NewWithID(applicationID)
}
