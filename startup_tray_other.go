//go:build !windows

package main

import "fyne.io/fyne/v2"

func windowsStartupControls() (fyne.CanvasObject, func() error) {
	return nil, nil
}
