package main

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) mainMenu() *fyne.MainMenu {
	return fyne.NewMainMenu(ui.settingsMenu(), ui.helpMenu())
}

func (ui *analyzerUI) helpMenu() *fyne.Menu {
	releases, _ := url.Parse("https://github.com/Losobie/evela/releases")
	return fyne.NewMenu("Help",
		fyne.NewMenuItem("About", ui.openAbout),
		fyne.NewMenuItem("Releases", func() {
			if err := fyne.CurrentApp().OpenURL(releases); err != nil {
				dialog.ShowError(err, ui.window)
				reportRuntimeError("Open link", err)
			}
		}),
		fyne.NewMenuItem("Licenses", ui.openLicenses),
		fyne.NewMenuItem("Error log", ui.openErrorLog),
	)
}

func (ui *analyzerUI) openAbout() {
	profile, _ := url.Parse("https://github.com/Losobie")
	author := widget.NewHyperlink("Losobie on GitHub", profile)
	character := widget.NewLabel(`In-game character: "Clento Loso"`)
	note := widget.NewLabel("This application was vibe coded using Codex.")
	note.Wrapping = fyne.TextWrapWord
	viewer := dialog.NewCustom("About", "Close", container.NewVBox(author, character, note), ui.window)
	viewer.Resize(fyne.NewSize(460, 220))
	viewer.Show()
}
