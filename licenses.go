package main

import (
	_ "embed"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Keep both texts in standalone exe downloads as well as source distributions.
//
//go:embed LICENSE
var projectLicense string

//go:embed THIRD_PARTY_NOTICES.txt
var thirdPartyNotices string

const licenseSectionSeparator = "\n\n========================================================================\n"

type licenseDocument struct{ name, text string }

func licenseDocuments() []licenseDocument {
	documents := []licenseDocument{{"Evela (MIT)", projectLicense}}
	for _, section := range strings.Split(thirdPartyNotices, licenseSectionSeparator) {
		name, _, _ := strings.Cut(section, "\n")
		documents = append(documents, licenseDocument{name, section})
	}
	return documents
}

func (ui *analyzerUI) openLicenses() {
	documents := licenseDocuments()
	names := make([]string, len(documents))
	for i, document := range documents {
		names[i] = document.name
	}
	text := widget.NewLabel("")
	text.Selectable, text.Wrapping = true, fyne.TextWrapWord
	scroll := container.NewVScroll(text)
	choose := widget.NewSelect(names, func(name string) {
		for _, document := range documents {
			if document.name == name {
				text.SetText(document.text)
				scroll.ScrollToTop()
				return
			}
		}
	})
	choose.SetSelectedIndex(0)
	note := widget.NewLabel("Evela's original code is MIT licensed. Dependencies, fonts and CCP game data retain their own terms.")
	note.Wrapping = fyne.TextWrapWord
	copyAll := widget.NewButtonWithIcon("Copy all licenses", theme.ContentCopyIcon(), func() {
		ui.window.Clipboard().SetContent(projectLicense + "\n\n" + thirdPartyNotices)
	})
	content := container.NewBorder(container.NewVBox(note, choose), copyAll, nil, nil, scroll)
	viewer := dialog.NewCustom("Licenses", "Close", content, ui.window)
	viewer.Resize(fyne.NewSize(1000, 700))
	viewer.Show()
}
