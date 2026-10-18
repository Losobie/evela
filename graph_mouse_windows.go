//go:build windows

package main

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"golang.org/x/sys/windows"
)

const (
	wmGraphXButtonDown   = 0x020B
	wmGraphXButtonUp     = 0x020C
	wmGraphXButtonDouble = 0x020D
	wmGraphAppCommand    = 0x0319
	wmGraphNCDestroy     = 0x0082
	graphMouseSubclassID = 0x4556454C
)

var (
	graphMouseControls  = windows.NewLazySystemDLL("comctl32.dll")
	graphSetSubclass    = graphMouseControls.NewProc("SetWindowSubclass")
	graphDefSubclass    = graphMouseControls.NewProc("DefSubclassProc")
	graphRemoveSubclass = graphMouseControls.NewProc("RemoveWindowSubclass")
	graphMouseHandlers  sync.Map // HWND -> func(redo bool); no Go pointers stored in Windows.
	graphMouseCallback  uintptr
)

func init() { graphMouseCallback = windows.NewCallback(graphMouseWindowProc) }

// Fyne 2.8 only forwards the three main mouse buttons. Intercept navigation
// messages on our own HWND, preserving GLFW's handling of all other messages.
// OnStarted runs on the window's owning OS thread, as SetWindowSubclass requires.
func (ui *analyzerUI) initGraphMouseNavigation() {
	window, ok := ui.window.(driver.NativeWindow)
	if !ok {
		return
	}
	window.RunNative(func(context any) {
		windowsContext, ok := context.(driver.WindowsWindowContext)
		if !ok {
			return
		}
		if err := installGraphMouseNavigation(windowsContext.HWND, func(redo bool) {
			// Native callbacks queue UI work rather than re-entering Fyne mid-event.
			fyne.Do(func() {
				select {
				case <-ui.stop:
					return
				default:
					ui.navigateGraphRange(redo)
				}
			})
		}); err != nil {
			fyne.LogError("Enable mouse Back/Forward for graph selections", err)
		}
	})
}

func installGraphMouseNavigation(hwnd uintptr, navigate func(bool)) error {
	if hwnd == 0 || navigate == nil {
		return fmt.Errorf("mouse navigation requires a native window and callback")
	}
	result, _, _ := graphSetSubclass.Call(hwnd, graphMouseCallback, graphMouseSubclassID, 0)
	if result == 0 {
		return fmt.Errorf("could not install graph mouse navigation on the window's owning thread")
	}
	graphMouseHandlers.Store(hwnd, navigate)
	return nil
}

func graphMouseWindowProc(hwnd, message, wParam, lParam, subclassID, _ uintptr) uintptr {
	if message == wmGraphNCDestroy {
		graphMouseHandlers.Delete(hwnd)
		graphRemoveSubclass.Call(hwnd, graphMouseCallback, subclassID)
	} else if handler, ok := graphMouseHandlers.Load(hwnd); ok {
		switch message {
		case wmGraphXButtonDown, wmGraphXButtonUp, wmGraphXButtonDouble:
			button := (wParam >> 16) & 0xffff
			if button == 1 || button == 2 {
				// Consume the entire click, dispatching once on release. Passing
				// it through could produce a duplicate browser app-command event.
				if message == wmGraphXButtonUp {
					handler.(func(bool))(button == 2)
				}
				return 1
			}
		case wmGraphAppCommand:
			// Some mouse drivers emit browser commands instead of XBUTTON events.
			command := (lParam >> 16) & 0x0fff
			if command == 1 || command == 2 {
				handler.(func(bool))(command == 2)
				return 1
			}
		}
	}
	result, _, _ := graphDefSubclass.Call(hwnd, message, wParam, lParam)
	return result
}
