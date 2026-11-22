//go:build !windows

package main

import "fyne.io/fyne/v2"

func windowsStartupControls(fyne.Window) fyne.CanvasObject {
	return nil
}
