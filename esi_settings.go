package main

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (ui *analyzerUI) initESISettings() {
	ui.callback = widget.NewEntry()
	ui.callback.SetPlaceHolder(esiCallbackURL(defaultESICallbackPort) + " (default)")
	ui.callback.Validator = func(value string) error { _, err := parseESICallback(value); return err }
	ui.clientID.OnChanged = func(string) {
		if !ui.settingESI {
			ui.esiSettingsDirty = true
		}
	}
	ui.callback.OnChanged = func(string) {
		if !ui.settingESI {
			ui.esiSettingsDirty = true
		}
	}
	ui.tokenConnections = container.NewVBox(widget.NewLabel("No saved character tokens yet."))
}

func parseESICallback(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultESICallbackPort, nil
	}
	parsed, err := url.Parse(value)
	if err == nil {
		port, portErr := parseCallbackPort(parsed.Port())
		if portErr == nil && value == esiCallbackURL(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("callback must be http://localhost:<port>/callback, with a port from 1 to 65535")
}

func (ui *analyzerUI) loadESIConfiguration(clientID string, port int) {
	ui.settingESI = true
	defer func() { ui.settingESI = false }()
	if clientID == defaultESIClientID {
		clientID = ""
	}
	ui.clientID.SetText(clientID)
	callback := ""
	if port != defaultESICallbackPort {
		callback = esiCallbackURL(port)
	}
	ui.callback.SetText(callback)
}

func (ui *analyzerUI) setESIStatus(message string) {
	ui.esiStatus.SetText(message)
}

func (ui *analyzerUI) requestESIConfiguration(kind string) {
	port, err := parseESICallback(ui.callback.Text)
	if err != nil {
		ui.setESIStatus(err.Error())
		return
	}
	clientID := strings.TrimSpace(ui.clientID.Text)
	if clientID == "" {
		clientID = defaultESIClientID
	}
	action := esiAction{Kind: kind, ClientID: clientID, CallbackPort: port}
	if kind == "connect" || kind == "connect-structures" {
		ui.requestESILogin(action)
		return
	}
	if ui.requestESI(action) {
		ui.setESIStatus("ESI request queued…")
	}
}

func (ui *analyzerUI) openESISettings() {
	if ui.esiDialog != nil {
		ui.esiDialog.Show()
		return
	}
	portal, _ := url.Parse("https://developers.eveonline.com/applications")
	applicationForm := widget.NewForm(widget.NewFormItem("Client ID", ui.clientID), widget.NewFormItem("Callback", ui.callback))
	application := container.NewVScroll(container.NewVBox(
		applicationForm,
		widget.NewLabel("Scopes:\n"+shipScope+"\n"+locationScope+"\n"+structureScope), widget.NewHyperlink("Open EVE developer applications", portal),
		widget.NewButton("Save application configuration", func() { ui.requestESIConfiguration("configure") })))
	signInNote := widget.NewLabel("Sign in with EVE Online for ship and location access. To also resolve player-owned structure names, first enable " + structureScope + " in your EVE application's registration, then choose Sign in for structure names. If that sign-in reports invalid_scope, use the normal sign-in until the registration is updated. Tokens are received and renewed automatically.")
	signInNote.Wrapping = fyne.TextWrapWord
	signInControls := container.NewVBox(signInNote, container.NewHBox(ui.connect, ui.connectStructures, ui.cancelLogin), widget.NewSeparator(), widget.NewLabel("SAVED CHARACTER TOKENS"))
	tokens := container.NewBorder(signInControls, nil, nil, nil, container.NewVScroll(ui.tokenConnections))
	ui.esiSettingsTabs = container.NewAppTabs(container.NewTabItem("Application", application), container.NewTabItem("Character tokens", tokens))
	security := widget.NewLabel("Sign-ins are saved encrypted for your Windows user. Saved token values are never displayed here.")
	security.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(nil, security, nil, nil, ui.esiSettingsTabs)
	ui.esiDialog = dialog.NewCustom("ESI configuration", "Close", content, ui.window)
	ui.esiDialog.SetOnClosed(func() { ui.esiDialog = nil })
	ui.esiDialog.Resize(fyne.NewSize(920, 720))
	ui.esiDialog.Show()
}
