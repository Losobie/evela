package main

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) mainMenu() *fyne.MainMenu {
	return fyne.NewMainMenu(ui.settingsMenu(), ui.aboutMenu())
}

func (ui *analyzerUI) aboutMenu() *fyne.Menu {
	repository, _ := url.Parse("https://github.com/Losobie/evela")
	return fyne.NewMenu("About",
		fyne.NewMenuItem("GitHub", func() {
			if err := fyne.CurrentApp().OpenURL(repository); err != nil {
				dialog.ShowError(err, ui.window)
				reportRuntimeError("Open link", err)
			}
		}),
		fyne.NewMenuItem("Author", ui.openAuthor),
		fyne.NewMenuItem("Licenses", ui.openLicenses),
	)
}

func (ui *analyzerUI) openAuthor() {
	profile, _ := url.Parse("https://github.com/Losobie")
	author := widget.NewHyperlink("Losobie on GitHub", profile)
	character := widget.NewLabel(`In-game character: "Clento Loso"`)
	note := widget.NewLabel("This application was vibe coded using Codex.")
	note.Wrapping = fyne.TextWrapWord
	viewer := dialog.NewCustom("Author", "Close", container.NewVBox(author, character, note), ui.window)
	viewer.Resize(fyne.NewSize(460, 220))
	viewer.Show()
}
