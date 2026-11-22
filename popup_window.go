package main

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Popup windows are created, shown and closed on the UI thread.
var openPopupWindows = make(map[fyne.Canvas][]*popupWindow)

type popupWindow struct {
	fyne.Window
	parent   fyne.Window
	shown    bool
	onClosed func()
}

func newPopupWindow(title string, content fyne.CanvasObject, parent fyne.Window) *popupWindow {
	popup := &popupWindow{Window: fyne.CurrentApp().NewWindow(title), parent: parent}
	popup.SetContent(container.NewPadded(content))
	popup.SetFixedSize(true)
	popup.Window.SetOnClosed(func() {
		closePopupWindows(popup)
		key := parent.Canvas()
		windows := slices.DeleteFunc(openPopupWindows[key], func(open *popupWindow) bool { return open == popup })
		if len(windows) == 0 {
			delete(openPopupWindows, key)
		} else {
			openPopupWindows[key] = windows
		}
		if popup.onClosed != nil {
			popup.onClosed()
		}
	})
	return popup
}

func (popup *popupWindow) SetOnClosed(onClosed func()) {
	popup.onClosed = onClosed
}

func (popup *popupWindow) Show() {
	if !popup.shown {
		popup.shown = true
		key := popup.parent.Canvas()
		openPopupWindows[key] = append(openPopupWindows[key], popup)
		main := popup.parent
		for {
			parent, ok := main.(*popupWindow)
			if !ok {
				break
			}
			main = parent.parent
		}
		showCenteredOnWindow(popup.Window, main)
	}
	popup.RequestFocus()
}

func lastPopupWindow(parent fyne.Window) *popupWindow {
	windows := openPopupWindows[parent.Canvas()]
	if len(windows) == 0 {
		return nil
	}
	return windows[len(windows)-1]
}

func closePopupWindows(parent fyne.Window) {
	for popup := lastPopupWindow(parent); popup != nil; popup = lastPopupWindow(parent) {
		popup.Close()
	}
}

func hidePopupWindows(parent fyne.Window) {
	for _, popup := range openPopupWindows[parent.Canvas()] {
		hidePopupWindows(popup)
		popup.Hide()
	}
}

func restorePopupWindows(parent fyne.Window) {
	for _, popup := range openPopupWindows[parent.Canvas()] {
		popup.Window.Show()
		restorePopupWindows(popup)
	}
}

func showPopupError(err error, parent fyne.Window) {
	message := widget.NewLabel(err.Error())
	message.Selectable = true
	message.Wrapping = fyne.TextWrapWord
	popup := newPopupWindow("Error", container.NewVScroll(message), parent)
	popup.Resize(fyne.NewSize(480, 180))
	popup.Show()
}
