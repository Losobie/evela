//go:build !windows

package main

import "fyne.io/fyne/v2"

// Fyne exposes window size on other platforms, but not its current position.
func (ui *analyzerUI) restoreWindowGeometry() {
	prefs := fyne.CurrentApp().Preferences()
	width, height := prefs.Float("window.width"), prefs.Float("window.height")
	if width > 0 && height > 0 {
		ui.window.Resize(fyne.NewSize(float32(width), float32(height)))
	}
}

func (ui *analyzerUI) saveWindowGeometry() {
	size := ui.window.Canvas().Size()
	prefs := fyne.CurrentApp().Preferences()
	prefs.SetFloat("window.width", float64(size.Width))
	prefs.SetFloat("window.height", float64(size.Height))
}
