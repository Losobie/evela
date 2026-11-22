package main

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDialogFrameKeepsCloseAndDropsMinMax(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class := windows.StringToUTF16Ptr("Static")
	const (
		styleIndex = ^uintptr(15)
		caption    = 0x00c00000
		systemMenu = 0x00080000
		minMax     = 0x00030000
	)
	hwnd, _, err := trayCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, caption|systemMenu|minMax, 0, 0, 350, 180, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	defer trayDestroyWindow.Call(hwnd)
	removeDialogMinMaxButtons(hwnd)
	style, _, _ := getWindowLong.Call(hwnd, styleIndex)
	if style&(caption|systemMenu|minMax) != caption|systemMenu {
		t.Fatalf("dialog style %#x must retain its title bar and close button without minimize or maximize", style)
	}
}
