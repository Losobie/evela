//go:build windows

package main

import (
	"reflect"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/desktop"
	"golang.org/x/sys/windows"
)

type graphNativeTestWindow struct {
	fyne.Window
	hwnd uintptr
}

func (window *graphNativeTestWindow) RunNative(callback func(any)) {
	callback(driver.WindowsWindowContext{HWND: window.hwnd})
}

// A message-only window receives synthetic messages without displaying a window,
// moving the pointer, or sending input to another application.
func newGraphMouseMessageWindow(t *testing.T) (uintptr, func(), func(uintptr, uintptr, uintptr) uintptr) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	user32 := windows.NewLazySystemDLL("user32.dll")
	create := user32.NewProc("CreateWindowExW")
	destroy := user32.NewProc("DestroyWindow")
	send := user32.NewProc("SendMessageW")
	class, _ := windows.UTF16PtrFromString("STATIC")
	title, _ := windows.UTF16PtrFromString("Graph navigation test")
	hwnd, _, err := create.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0) // HWND_MESSAGE == -3.
	if hwnd == 0 {
		t.Fatalf("create message-only test window: %v", err)
	}
	destroyed := false
	cleanup := func() {
		if !destroyed {
			destroy.Call(hwnd)
			destroyed = true
		}
	}
	t.Cleanup(cleanup)
	return hwnd, cleanup, func(message, wParam, lParam uintptr) uintptr {
		result, _, _ := send.Call(hwnd, message, wParam, lParam)
		return result
	}
}

func TestGraphMouseNativeMessagesAndCleanup(t *testing.T) {
	hwnd, destroy, send := newGraphMouseMessageWindow(t)
	var directions []bool
	if err := installGraphMouseNavigation(hwnd, func(redo bool) { directions = append(directions, redo) }); err != nil {
		t.Fatal(err)
	}
	for _, message := range []uintptr{wmGraphXButtonDown, wmGraphXButtonDouble} {
		if send(message, 1<<16, 0) != 1 || len(directions) != 0 {
			t.Fatal("navigation fired before release or the side-button message was not consumed")
		}
	}
	if send(wmGraphXButtonUp, 1<<16|0x0008, 0) != 1 { // Low-word Ctrl modifier must not change the button.
		t.Fatal("Back release was not consumed")
	}
	if send(wmGraphXButtonUp, 2<<16, 0) != 1 {
		t.Fatal("Forward release was not consumed")
	}
	// Browser app commands from mouse drivers carry a device flag in the high word.
	send(wmGraphAppCommand, hwnd, (0x8000|1)<<16)
	send(wmGraphAppCommand, hwnd, (0x8000|2)<<16)
	if !reflect.DeepEqual(directions, []bool{false, true, false, true}) {
		t.Fatalf("wrong or duplicate navigation events: %v", directions)
	}
	if length := send(0x000E, 0, 0); length != uintptr(len("Graph navigation test")) { // WM_GETTEXTLENGTH.
		t.Fatal("unrelated window messages did not reach the original window procedure")
	}
	destroy()
	if _, exists := graphMouseHandlers.Load(hwnd); exists {
		t.Fatal("destroyed window retained its navigation callback")
	}
}

func TestGraphMouseBackForwardSharesShortcutHistory(t *testing.T) {
	hwnd, _, send := newGraphMouseMessageWindow(t)
	ui, start := newGraphHistoryTestUI(t)
	ui.window = &graphNativeTestWindow{Window: ui.window, hwnd: hwnd}
	ui.initGraphMouseNavigation()
	initial := graphHistoryState(ui)
	send(wmGraphXButtonUp, 1<<16, 0)
	send(wmGraphXButtonUp, 2<<16, 0)
	assertGraphHistoryState(t, ui, initial, 1180)
	dragGraphFraction(ui.outgoing, .25, .9)
	first := graphHistoryState(ui)
	dragGraphFraction(ui.incoming, .1, .6)
	second := graphHistoryState(ui)
	ui.graphLogarithmic, ui.graphAverage10s = true, true
	ui.renderSelection()
	send(wmGraphXButtonDown, 1<<16, 0)
	assertGraphHistoryState(t, ui, second, 30)
	send(wmGraphXButtonUp, 1<<16, 0)
	assertGraphHistoryState(t, ui, first, 90)
	sendGraphShortcut(ui, &fyne.ShortcutUndo{})
	assertGraphHistoryState(t, ui, initial, 1180)
	send(wmGraphXButtonUp, 2<<16, 0)
	assertGraphHistoryState(t, ui, first, 90)
	sendGraphShortcut(ui, &fyne.ShortcutRedo{})
	assertGraphHistoryState(t, ui, second, 30)
	if len(ui.graphUndo) != 2 || len(ui.graphRedo) != 0 || !ui.graphAverage10s || !ui.outgoing.data.Logarithmic {
		t.Fatal("mouse navigation duplicated history or changed graph formatting")
	}
	ui.window.Canvas().Focus(ui.characterSearch)
	send(wmGraphXButtonUp, 1<<16, 0)
	assertGraphHistoryState(t, ui, second, 30)
	ui.window.Canvas().Unfocus()
	ui.openESISettings()
	send(wmGraphXButtonUp, 1<<16, 0)
	assertGraphHistoryState(t, ui, second, 30)
	ui.esiDialog.Hide()
	ui.graphFormatButton.OnTapped()
	send(wmGraphXButtonUp, 1<<16, 0)
	assertGraphHistoryState(t, ui, second, 30)
	ui.window.Canvas().Overlays().Remove(ui.window.Canvas().Overlays().Top())
	// Undo during an unfinished selection must cancel it rather than add a new zoom.
	chart := ui.incoming
	width := chart.Size().Width - chartPlotLeft - chartPlotRight
	from, to := fyne.NewPos(chartPlotLeft+width*.2, 80), fyne.NewPos(chartPlotLeft+width*.8, 80)
	chart.MouseDown(testMouseEvent(from.X, from.Y, desktop.MouseButtonPrimary))
	chart.Dragged(testDragEvent(from, to))
	send(wmGraphXButtonUp, 1<<16, 0)
	chart.DragEnd()
	assertGraphHistoryState(t, ui, first, 90)
	send(wmGraphXButtonUp, 2<<16, 0)
	assertGraphHistoryState(t, ui, second, 30)
	// A fresh drag starts a branch, just as with keyboard undo/redo.
	send(wmGraphXButtonUp, 1<<16, 0)
	ui.applyGraphRange(start.Add(35*time.Minute), start.Add(50*time.Minute))
	branch := graphHistoryState(ui)
	send(wmGraphXButtonUp, 2<<16, 0)
	assertGraphHistoryState(t, ui, branch, 60)
	ui.panGraphRange(true)
	pan := graphHistoryState(ui)
	send(wmGraphXButtonUp, 1<<16, 0)
	assertGraphHistoryState(t, ui, branch, 60)
	send(wmGraphXButtonUp, 2<<16, 0)
	assertGraphHistoryState(t, ui, pan, 60)
	scrollGraph(ui.outgoing, .5, -25)
	scrollGraph(ui.incoming, .5, -25)
	wheel := graphHistoryState(ui)
	send(wmGraphXButtonUp, 1<<16, 0)
	assertGraphHistoryState(t, ui, pan, 60)
	send(wmGraphXButtonUp, 2<<16, 0)
	assertGraphHistoryState(t, ui, wheel, 60)
}
