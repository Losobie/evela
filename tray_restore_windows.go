package main

import (
	"log"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	trayMessageOnlyParent = ^uintptr(2) // HWND_MESSAGE
	trayRestoreMessage    = 0x8000      // WM_APP, private to this window class
)

type trayWindowClass struct {
	style                              uint32
	windowProc                         uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menuName, className                *uint16
}

var (
	trayRestoreClassName     = windows.StringToUTF16Ptr(applicationID + ".TrayRestore")
	trayRegisterClass        = windowUser32.NewProc("RegisterClassW")
	trayCreateWindow         = windowUser32.NewProc("CreateWindowExW")
	trayDestroyWindow        = windowUser32.NewProc("DestroyWindow")
	trayFindWindow           = windowUser32.NewProc("FindWindowExW")
	trayDefaultWindowProc    = windowUser32.NewProc("DefWindowProcW")
	traySendMessage          = windowUser32.NewProc("SendMessageTimeoutW")
	trayGetWindowProcess     = windowUser32.NewProc("GetWindowThreadProcessId")
	trayAllowForeground      = windowUser32.NewProc("AllowSetForegroundWindow")
	trayRestoreAction        func() // Read and written on the window's owning UI thread.
	registerTrayRestoreClass = sync.OnceValues(func() (windows.Handle, error) {
		return registerTrayWindowClass(trayRestoreClassName)
	})
)

func restoreTrayInstance() bool {
	return requestTrayRestore(trayRestoreClassName)
}

func requestTrayRestore(className *uint16) bool {
	var previous uintptr
	for {
		hwnd, _, _ := trayFindWindow.Call(trayMessageOnlyParent, previous, uintptr(unsafe.Pointer(className)), 0)
		if hwnd == 0 || hwnd == previous {
			return false
		}
		var pid uint32
		trayGetWindowProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != 0 {
			trayAllowForeground.Call(uintptr(pid))
		}
		if sendTrayRestore(hwnd) {
			return true
		}
		previous = hwnd
	}
}

func sendTrayRestore(hwnd uintptr) bool {
	const flags = 0x0001 | 0x0002 | 0x0020 // Block, abort if hung, fail if the window closes.
	var accepted uintptr
	ok, _, _ := traySendMessage.Call(hwnd, trayRestoreMessage, 0, 0, flags, 500, uintptr(unsafe.Pointer(&accepted)))
	return ok != 0 && accepted == 1
}

func registerTrayWindowClass(name *uint16) (windows.Handle, error) {
	var module windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &module); err != nil {
		return 0, err
	}
	class := trayWindowClass{windowProc: windows.NewCallback(trayRestoreWindowProc), instance: uintptr(module), className: name}
	if atom, _, err := trayRegisterClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return 0, err
	}
	return module, nil
}

func createTrayRestoreWindow(name *uint16, module windows.Handle) (uintptr, error) {
	hwnd, _, err := trayCreateWindow.Call(0, uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, trayMessageOnlyParent, 0, uintptr(module), 0)
	if hwnd == 0 {
		return 0, err
	}
	return hwnd, nil
}

// A message-only window exists only while Evela is hidden in the tray.
// GLFW's Windows event loop dispatches its messages on the UI thread.
func listenForTrayRestore(restore func()) func() {
	module, err := registerTrayRestoreClass()
	if err != nil {
		log.Printf("Register tray restore window: %v", err)
		return nil
	}
	hwnd, err := createTrayRestoreWindow(trayRestoreClassName, module)
	if err != nil {
		log.Printf("Create tray restore window: %v", err)
		return nil
	}
	trayRestoreAction = restore
	return func() {
		trayRestoreAction = nil
		trayDestroyWindow.Call(hwnd)
	}
}

func trayRestoreWindowProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if message == trayRestoreMessage {
		if restore := trayRestoreAction; restore != nil {
			restore()
			return 1
		}
		return 0
	}
	result, _, _ := trayDefaultWindowProc.Call(hwnd, message, wParam, lParam)
	return result
}
