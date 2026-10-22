package main

import (
	"net/url"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) initESISettings() {
	ui.callbackPort = widget.NewEntry()
	ui.callbackPort.SetText(strconv.Itoa(defaultESICallbackPort))
	ui.callbackPort.Validator = func(value string) error { _, err := parseCallbackPort(value); return err }
	ui.callbackURL = widget.NewLabel(esiCallbackURL(defaultESICallbackPort))
	ui.callbackURL.TextStyle.Monospace = true
	ui.callbackURL.Wrapping = fyne.TextWrapWord
	ui.clientID.OnChanged = func(string) {
		if !ui.settingESI {
			ui.esiSettingsDirty = true
		}
	}
	ui.callbackPort.OnChanged = func(value string) {
		if !ui.settingESI {
			ui.esiSettingsDirty = true
		}
		port, err := parseCallbackPort(value)
		if err != nil {
			ui.callbackURL.SetText("Enter a port from 1 to 65535.")
		} else {
			ui.callbackURL.SetText(esiCallbackURL(port))
		}
	}
	ui.esiDialogStatus = widget.NewLabel("")
	ui.esiDialogStatus.Wrapping = fyne.TextWrapWord
	ui.esiDialogStatus.Truncation = fyne.TextTruncateEllipsis
	ui.tokenConnections = container.NewVBox(widget.NewLabel("No saved character tokens yet."))
}

func (ui *analyzerUI) setESIStatus(message string) {
	ui.esiStatus.SetText(message)
	ui.esiDialogStatus.SetText(message)
}

func (ui *analyzerUI) requestESIConfiguration(kind string) {
	port, err := parseCallbackPort(ui.callbackPort.Text)
	if err != nil {
		ui.setESIStatus(err.Error())
		return
	}
	clientID := strings.TrimSpace(ui.clientID.Text)
	if clientID == "" {
		ui.setESIStatus("Enter your EVE application's public Client ID.")
		return
	}
	action := esiAction{Kind: kind, ClientID: clientID, CallbackPort: port}
	if ui.requestESI(action) {
		ui.setESIStatus("ESI request queued…")
	}
}

func (ui *analyzerUI) openESISettings() {
	if ui.esiDialog != nil {
		ui.esiDialog.Show()
		return
	}
	intro := widget.NewLabel("Sign in with EVE Online using PKCE. The default application ID is filled in; you can enter your own native / desktop application's public Client ID.")
	intro.Wrapping = fyne.TextWrapWord
	callbackNote := widget.NewLabel("Register this exact callback URL in your EVE developer application. The hostname is localhost; the port defaults to 8080. Changing the port also changes the local sign-in listener.")
	callbackNote.Wrapping = fyne.TextWrapWord
	copyCallback := widget.NewButton("Copy callback URL", func() {
		port, err := parseCallbackPort(ui.callbackPort.Text)
		if err != nil {
			ui.setESIStatus(err.Error())
			return
		}
		ui.window.Clipboard().SetContent(esiCallbackURL(port))
	})
	portal, _ := url.Parse("https://developers.eveonline.com/applications")
	applicationForm := widget.NewForm(widget.NewFormItem("Client ID", ui.clientID), widget.NewFormItem("Callback port", ui.callbackPort))
	application := container.NewVScroll(container.NewVBox(intro,
		applicationForm,
		callbackNote, ui.callbackURL, copyCallback,
		widget.NewLabel("Ship scope: "+shipScope), widget.NewLabel("Location scope: "+locationScope), widget.NewHyperlink("Open EVE developer applications", portal),
		widget.NewButton("Save application configuration", func() { ui.requestESIConfiguration("configure") })))
	signInNote := widget.NewLabel("Sign in with EVE Online for each character to grant ship and location access. Tokens are received and renewed automatically. Existing ship-only sign-ins still work for ships; sign in again to grant location access.")
	signInNote.Wrapping = fyne.TextWrapWord
	signInControls := container.NewVBox(signInNote, container.NewHBox(ui.connect, ui.cancelLogin), widget.NewSeparator(), widget.NewLabel("SAVED CHARACTER TOKENS"))
	tokens := container.NewBorder(signInControls, nil, nil, nil, container.NewVScroll(ui.tokenConnections))
	ui.esiSettingsTabs = container.NewAppTabs(container.NewTabItem("Application", application), container.NewTabItem("Character tokens", tokens))
	security := widget.NewLabel("Sign-ins are saved encrypted for your Windows user. Saved token values are never displayed here.")
	security.Wrapping = fyne.TextWrapWord
	ui.esiDialogStatus.SetText(ui.esiStatus.Text)
	content := container.NewBorder(nil, container.NewVBox(widget.NewSeparator(), ui.esiStatusRow(ui.esiDialogStatus), security), nil, nil, ui.esiSettingsTabs)
	ui.esiDialog = dialog.NewCustom("ESI configuration", "Close", content, ui.window)
	ui.esiDialog.SetOnClosed(func() { ui.esiDialog = nil })
	ui.esiDialog.Resize(fyne.NewSize(920, 720))
	ui.esiDialog.Show()
}
