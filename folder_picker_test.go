package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestFolderPickerWindowSelectsAndDismisses(t *testing.T) {
	app := test.NewTempApp(t)
	main := app.NewWindow("Evela")
	defer main.Close()
	settings := newPopupWindow("Log loading", widget.NewLabel("Log folders"), main)
	settings.Show()
	defer closePopupWindows(main)

	var selected string
	open := func() *popupWindow {
		t.Helper()
		openFolderPicker(settings, func(path string) { selected = path })
		picker := lastPopupWindow(settings)
		if picker == nil || picker.Title() != "Open Folder" || !picker.FixedSize() || settings.Canvas().Overlays().Top() != nil {
			t.Fatal("the folder picker must open in its own window")
		}
		return picker
	}
	open().Close()
	if selected != "" || lastPopupWindow(settings) != nil {
		t.Fatal("dismissing the folder picker selected a folder or left it open")
	}

	picker := open()
	var address *widget.Entry
	var selectFolder *widget.Button
	walkFilterObjects(picker.Content(), func(object fyne.CanvasObject) {
		switch item := object.(type) {
		case *widget.Entry:
			address = item
		case *widget.Button:
			switch item.Text {
			case "Cancel", "Close":
				t.Errorf("found a redundant %q button", item.Text)
			case "Select folder":
				selectFolder = item
			}
		}
	})
	if address == nil || selectFolder == nil {
		t.Fatal("the picker is missing its path field or selection action")
	}
	path := t.TempDir()
	address.SetText(path)
	test.Tap(selectFolder)
	if selected != path || lastPopupWindow(settings) != nil {
		t.Fatalf("selecting a folder returned %q, want %q, and should close the picker", selected, path)
	}
	open()
	settings.Close()
	if lastPopupWindow(settings) != nil || lastPopupWindow(main) != nil {
		t.Fatal("closing the settings window left its folder picker open")
	}
}
