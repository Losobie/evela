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
		if item.Label == "ESI configuration…" {
			item.Action()
		}
	}
	if ui.esiDialog == nil || ui.callbackPort.Text != "8080" || ui.callbackURL.Text != "http://localhost:8080/callback" {
		t.Fatal("menu did not open default ESI settings dialog")
	}
	if ui.clientID.Text != defaultESIClientID || ui.esiSettingsDirty {
		t.Fatal("default application ID was not filled in cleanly")
	}
	ui.requestESIConfiguration("connect")
	defaultAction := <-ui.esiActions
	if defaultAction.ClientID != defaultESIClientID || defaultAction.CallbackPort != defaultESICallbackPort || defaultAction.Kind != "connect" {
		t.Fatal("default sign-in did not use the built-in application")
	}
	first := ui.esiDialog
	ui.openESISettings()
	if ui.esiDialog != first {
		t.Fatal("opening settings twice created two dialogs")
	}
	ui.clientID.SetText("example-client")
	ui.callbackPort.SetText("9000")
	if ui.callbackURL.Text != "http://localhost:9000/callback" {
		t.Fatal("callback preview did not track port")
	}
	ui.requestESIConfiguration("configure")
	action := <-ui.esiActions
	if action.ClientID != "example-client" || action.CallbackPort != 9000 || action.Kind != "configure" {
		t.Fatal("settings were not included in configure action")
	}
	ui.callbackPort.SetText("0")
	ui.requestESIConfiguration("connect")
	if len(ui.esiActions) != 0 {
		t.Fatal("invalid callback port queued a login")
	}
	ui.callbackPort.SetText("8080")
	ui.clientID.SetText("")
	ui.requestESIConfiguration("configure")
	if len(ui.esiActions) != 0 {
		t.Fatal("an empty application ID was accepted")
	}
	ui.clientID.SetText("example-client")
	test.Tap(ui.connect)
	action = <-ui.esiActions
	if action.Kind != "connect" || action.ClientID != "example-client" || action.CallbackPort != 8080 {
		t.Fatal("sign-in button did not use the configured application")
	}
	ui.setESIStatus("ESI settings ready")
	if ui.esiStatus.Text != ui.esiDialogStatus.Text {
		t.Fatal("dialog status not synchronized")
	}
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
		capture(path)
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
