//go:build windows

package main

import (
	"encoding/json"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"golang.org/x/sys/windows"
)

const windowGeometryPreference = "window.placement"

var (
	windowUser32       = windows.NewLazySystemDLL("user32.dll")
	getWindowPlacement = windowUser32.NewProc("GetWindowPlacement")
	setWindowPlacement = windowUser32.NewProc("SetWindowPlacement")
	getClientRect      = windowUser32.NewProc("GetClientRect")
	clientToScreen     = windowUser32.NewProc("ClientToScreen")
	getWindowRect      = windowUser32.NewProc("GetWindowRect")
	setWindowPos       = windowUser32.NewProc("SetWindowPos")
)

type windowPoint struct{ X, Y int32 }
type windowRect struct{ Left, Top, Right, Bottom int32 }

// Matches Win32 WINDOWPLACEMENT. Keep its workspace coordinates together:
// mixing these with screen coordinates makes the window drift near taskbars.
type windowPlacement struct {
	Length         uint32      `json:"-"`
	Flags          uint32      `json:"flags,omitempty"`
	ShowCmd        uint32      `json:"show"`
	MinPosition    windowPoint `json:"-"`
	MaxPosition    windowPoint `json:"-"`
	NormalPosition windowRect  `json:"normal"`
}

func (placement windowPlacement) valid() bool {
	r := placement.NormalPosition
	return r.Right > r.Left && r.Bottom > r.Top
}

func savedWindowPlacement() (windowPlacement, bool) {
	var placement windowPlacement
	if json.Unmarshal([]byte(fyne.CurrentApp().Preferences().String(windowGeometryPreference)), &placement) != nil || !placement.valid() {
		return windowPlacement{}, false
	}
	// Never reopen minimized or hidden. Retain the normal bounds even when the
	// window was maximized, so restoring it later returns to its previous size.
	if placement.ShowCmd == 3 || (placement.ShowCmd == 2 && placement.Flags&2 != 0) {
		placement.ShowCmd = 3 // SW_SHOWMAXIMIZED
	} else {
		placement.ShowCmd = 1 // SW_SHOWNORMAL
	}
	placement.Length = uint32(unsafe.Sizeof(placement))
	placement.Flags = 0
	placement.MinPosition, placement.MaxPosition = windowPoint{-1, -1}, windowPoint{-1, -1}
	return placement, true
}

// Called from Fyne's OnStarted / OnClosed callbacks on the native window thread.
func (ui *analyzerUI) restoreWindowGeometry() {
	placement, ok := savedWindowPlacement()
	if !ok {
		return
	}
	ui.withNativeWindowHandle(func(hwnd uintptr) {
		// Windows adjusts off-screen placements if a monitor was disconnected.
		result, _, err := setWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement)))
		if result == 0 {
			fyne.LogError("Restore window size and position", err)
		}
	})
}

func (ui *analyzerUI) saveWindowGeometry() {
	ui.withNativeWindowHandle(func(hwnd uintptr) {
		placement := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
		result, _, err := getWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement)))
		if result == 0 {
			fyne.LogError("Save window size and position", err)
			return
		}
		if placement.valid() {
			data, _ := json.Marshal(placement)
			fyne.CurrentApp().Preferences().SetString(windowGeometryPreference, string(data))
		}
	})
}

func (ui *analyzerUI) withNativeWindowHandle(action func(uintptr)) {
	if hwnd := nativeWindowHandle(ui.window); hwnd != 0 {
		action(hwnd)
	}
}

func nativeWindowHandle(window fyne.Window) (hwnd uintptr) {
	if window, ok := window.(driver.NativeWindow); ok {
		window.RunNative(func(context any) {
			if context, ok := context.(driver.WindowsWindowContext); ok {
				hwnd = context.HWND
			}
		})
	}
	return hwnd
}

func showCenteredOnClient(prompt, parent fyne.Window) {
	// Fyne creates the native window and sets its frame size on Show.
	prompt.CenterOnScreen()
	prompt.Show()

	parentHandle, promptHandle := nativeWindowHandle(parent), nativeWindowHandle(prompt)
	if parentHandle == 0 || promptHandle == 0 {
		return
	}
	var client windowRect
	ok, _, _ := getClientRect.Call(parentHandle, uintptr(unsafe.Pointer(&client)))
	if ok == 0 {
		return
	}
	center := windowPoint{X: client.Right / 2, Y: client.Bottom / 2}
	ok, _, _ = clientToScreen.Call(parentHandle, uintptr(unsafe.Pointer(&center)))
	if ok == 0 {
		return
	}
	const moveOnly = 0x0001 | 0x0004 | 0x0010 // SWP_NOSIZE | SWP_NOZORDER | SWP_NOACTIVATE
	// A move between monitors can resize the frame when its DPI changes.
	for range 2 {
		var frame windowRect
		ok, _, _ = getWindowRect.Call(promptHandle, uintptr(unsafe.Pointer(&frame)))
		if ok == 0 {
			return
		}
		x := center.X - (frame.Right-frame.Left)/2
		y := center.Y - (frame.Bottom-frame.Top)/2
		if frame.Left == x && frame.Top == y {
			return
		}
		ok, _, _ = setWindowPos.Call(promptHandle, 0, uintptr(x), uintptr(y), 0, 0, moveOnly)
		if ok == 0 {
			return
		}
	}
}
