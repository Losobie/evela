//go:build windows

package main

import (
	"encoding/json"
	"testing"
	"unsafe"

	"fyne.io/fyne/v2/test"
)

func TestSavedWindowPlacementValidationAndState(t *testing.T) {
	application := test.NewTempApp(t)
	prefs := application.Preferences()
	for _, value := range []string{"", "broken", "null", "{}", `{"normal":{"Left":10,"Top":20,"Right":10,"Bottom":100}}`} {
		prefs.SetString(windowGeometryPreference, value)
		if _, ok := savedWindowPlacement(); ok {
			t.Fatalf("invalid saved placement was accepted: %s", value)
		}
	}
	for _, tc := range []struct {
		name        string
		show, flags uint32
		wantShow    uint32
	}{
		{"normal", 1, 0, 1},
		{"maximized", 3, 0, 3},
		{"minimized", 2, 0, 1},
		{"minimized from maximized", 2, 2, 3},
		{"hidden", 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Negative coordinates are valid for a monitor left of the primary.
			bounds := windowRect{-1500, -100, -100, 800}
			data, err := json.Marshal(windowPlacement{Flags: tc.flags, ShowCmd: tc.show, NormalPosition: bounds})
			if err != nil {
				t.Fatal(err)
			}
			prefs.SetString(windowGeometryPreference, string(data))
			placement, ok := savedWindowPlacement()
			if !ok || placement.NormalPosition != bounds || placement.ShowCmd != tc.wantShow || placement.Flags != 0 {
				t.Fatalf("wrong restored geometry or window state: %+v", placement)
			}
			if placement.Length != 44 || placement.MinPosition != (windowPoint{-1, -1}) || placement.MaxPosition != (windowPoint{-1, -1}) {
				t.Fatal("placement does not match the Win32 ABI or retains stale monitor positions")
			}
		})
	}
}

func TestWindowGeometryNativePreferenceRoundTrip(t *testing.T) {
	application := test.NewTempApp(t)
	hwnd, _, _ := newGraphMouseMessageWindow(t)
	ui := &analyzerUI{window: &graphNativeTestWindow{hwnd: hwnd}}
	set := func(bounds windowRect) {
		t.Helper()
		placement := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{})), NormalPosition: bounds}
		// HWND_MESSAGE windows remain invisible, even during restoration.
		if result, _, err := setWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement))); result == 0 {
			t.Fatalf("set native test geometry: %v", err)
		}
	}
	read := func() windowPlacement {
		t.Helper()
		placement := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
		if result, _, err := getWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement))); result == 0 {
			t.Fatalf("read native test geometry: %v", err)
		}
		return placement
	}
	set(windowRect{80, 60, 1080, 760})
	want := read().NormalPosition
	ui.saveWindowGeometry()
	saved, ok := savedWindowPlacement()
	if !ok || saved.NormalPosition != want {
		t.Fatalf("native geometry was not saved as a user preference: %+v", saved)
	}
	set(windowRect{200, 150, 1000, 750})
	ui.restoreWindowGeometry()
	if got := read().NormalPosition; got != want {
		t.Fatalf("native window restored to %+v, want %+v", got, want)
	}
	// Corrupt preferences leave the current/default placement intact.
	application.Preferences().SetString(windowGeometryPreference, "broken")
	ui.restoreWindowGeometry()
	if got := read().NormalPosition; got != want {
		t.Fatal("invalid preferences changed the native window placement")
	}
}
