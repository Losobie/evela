package main

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	if kind == "connect" {
		for _, check := range ui.loginScopeChecks {
			if check.Checked {
				action.Scopes = append(action.Scopes, check.Text)
			}
		}
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
	ui.esiApplication = application
	ui.esiDialog = newPopupWindow("ESI configuration", application, ui.window)
	ui.esiDialog.SetOnClosed(func() { ui.esiDialog = nil })
	ui.esiDialog.Resize(fyne.NewSize(780, 420))
	ui.esiDialog.Show()
}
