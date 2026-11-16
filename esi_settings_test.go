package main

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestESIMenuDialogAndConfigurationActions(t *testing.T) {
	application := test.NewTempApp(t)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow("ESI settings preview")
	defer window.Close()
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	menu := window.MainMenu()
	if menu == nil || len(menu.Items) == 0 {
		t.Fatal("menu bar not installed")
	}
	for _, item := range menu.Items[0].Items {
		if item.Label == "ESI configuration" {
			item.Action()
		}
	}
	ui.loadESIConfiguration(defaultESIClientID, defaultESICallbackPort)
	if ui.esiDialog == nil || ui.callback.Text != "" || ui.callback.PlaceHolder != "http://localhost:8080/callback" {
		t.Fatal("menu did not open default ESI settings dialog")
	}
	if ui.clientID.Text != "" || ui.clientID.PlaceHolder != defaultESIClientID || ui.esiSettingsDirty {
		t.Fatal("default application ID was not shown as a placeholder")
	}
	ui.requestESIConfiguration("connect")
	defaultLogin := <-ui.esiLogins
	defer defaultLogin.cancel()
	defaultAction := defaultLogin.action
	if defaultAction.ClientID != defaultESIClientID || defaultAction.CallbackPort != defaultESICallbackPort || defaultAction.Kind != "connect" {
		t.Fatal("default sign-in did not use the built-in application")
	}
	if !ui.connect.Disabled() || !ui.connectStructures.Disabled() || !ui.cancelLogin.Visible() {
		t.Fatal("pending sign-in did not immediately expose cancellation and prevent duplicates")
	}
	ui.requestESIConfiguration("connect")
	if len(ui.esiLogins) != 0 {
		t.Fatal("a second login was queued while one was pending")
	}
	defaultLogin.cancel()
	ui.cancelESILogin = nil
	ui.connect.Enable()
	ui.connectStructures.Enable()
	ui.cancelLogin.Hide()
	first := ui.esiDialog
	ui.openESISettings()
	if ui.esiDialog != first {
		t.Fatal("opening settings twice created two dialogs")
	}
	ui.clientID.SetText("example-client")
	ui.callback.SetText("http://localhost:9000/callback")
	if err := ui.callback.Validate(); err != nil {
		t.Fatal("full callback URL was rejected:", err)
	}
	ui.requestESIConfiguration("configure")
	action := <-ui.esiActions
	if action.ClientID != "example-client" || action.CallbackPort != 9000 || action.Kind != "configure" {
		t.Fatal("settings were not included in configure action")
	}
	ui.callback.SetText("http://localhost:0/callback")
	ui.requestESIConfiguration("connect")
	if len(ui.esiActions) != 0 || len(ui.esiLogins) != 0 {
		t.Fatal("invalid callback port queued a login")
	}
	ui.callback.SetText("")
	ui.clientID.SetText("")
	ui.requestESIConfiguration("configure")
	action = <-ui.esiActions
	if action.ClientID != defaultESIClientID || action.CallbackPort != defaultESICallbackPort {
		t.Fatal("clearing overrides did not restore the defaults")
	}
	ui.clientID.SetText("example-client")
	test.Tap(ui.connect)
	login := <-ui.esiLogins
	defer login.cancel()
	action = login.action
	if action.Kind != "connect" || action.ClientID != "example-client" || action.CallbackPort != 8080 {
		t.Fatal("sign-in button did not use the configured application")
	}
	ui.setESIStatus("ESI settings ready")
	if path := os.Getenv("EVELA_SETTINGS_CAPTURE"); path != "" {
		capture := func(path string) {
			t.Helper()
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, window.Canvas().Capture()); err != nil {
				t.Fatal(err)
			}
		}
		clientID := ui.clientID.Text
		ui.clientID.SetText("")
		capture(path)
		ui.clientID.SetText(clientID)
		ui.esiSettingsTabs.SelectIndex(1)
		capture(path + ".tokens.png")
	}
	ui.esiDialog.Hide()
	if ui.esiDialog != nil {
		t.Fatal("closing the dialog did not release it")
	}
	ui.openESISettings()
	if ui.clientID.Text != "example-client" {
		t.Fatal("reopening lost application settings")
	}
	ui.esiDialog.Hide()
}

func TestESIFullCallbackValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		port  int
	}{
		{"", 8080}, {"  ", 8080}, {"http://localhost:8080/callback", 8080},
		{"http://localhost:9000/callback", 9000}, {" http://localhost:65535/callback ", 65535},
	} {
		if port, err := parseESICallback(tc.value); err != nil || port != tc.port {
			t.Errorf("callback %q = %d, %v; want port %d", tc.value, port, err, tc.port)
		}
	}
	for _, value := range []string{"9000", "http://localhost:0/callback", "http://localhost:65536/callback", "http://localhost:bad/callback", "https://localhost:8080/callback", "http://example.com:8080/callback", "http://localhost:8080/other", "http://localhost:8080/callback?extra=true", "http://user@localhost:8080/callback", "http://localhost:8080/callback#fragment"} {
		if _, err := parseESICallback(value); err == nil {
			t.Errorf("accepted unsupported callback %q", value)
		}
	}
}
