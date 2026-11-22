package main

import (
	"errors"
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	windowsStartupKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	windowsStartupValue = "Evela"
)

func windowsStartupControls(window fyne.Window) fyne.CanvasObject {
	enabled, minimized, err := readWindowsStartup()
	if err != nil {
		reportRuntimeError("Read Windows startup settings", err)
		return widget.NewLabel("Windows startup settings are unavailable.")
	}
	startMinimized := widget.NewCheck("Start minimized to system tray", nil)
	startAtSignIn := widget.NewCheck("Start when I sign in to Windows", nil)
	startMinimized.SetChecked(minimized)
	startAtSignIn.SetChecked(enabled)
	if !enabled {
		startMinimized.Disable()
	}
	apply := func(bool) {
		if startAtSignIn.Checked == enabled && startMinimized.Checked == minimized {
			return
		}
		if err := saveWindowsStartup(startAtSignIn.Checked, startMinimized.Checked); err != nil {
			startAtSignIn.SetChecked(enabled)
			startMinimized.SetChecked(minimized)
			reportRuntimeError("Save Windows startup settings", err)
			showPopupError(err, window)
			return
		}
		enabled, minimized = startAtSignIn.Checked, startMinimized.Checked
		if enabled {
			startMinimized.Enable()
		} else {
			startMinimized.Disable()
		}
	}
	startAtSignIn.OnChanged = apply
	startMinimized.OnChanged = apply
	return container.NewVBox(startAtSignIn, startMinimized)
}

func readWindowsStartup() (enabled, minimized bool, err error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsStartupKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	defer key.Close()
	command, _, err := key.GetStringValue(windowsStartupValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	args, err := windows.DecomposeCommandLine(command)
	if err != nil {
		return false, false, err
	}
	return len(args) > 0, len(args) == 2 && args[1] == startInTrayArg, nil
}

func saveWindowsStartup(enabled, minimized bool) error {
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, windowsStartupKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(windowsStartupValue)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	path, err := os.Executable()
	if err != nil {
		return err
	}
	command, err := windowsStartupCommand(path, minimized)
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, windowsStartupKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(windowsStartupValue, command)
}

func windowsStartupCommand(path string, minimized bool) (string, error) {
	args := []string{path}
	if minimized {
		args = append(args, startInTrayArg)
	}
	command := windows.ComposeCommandLine(args)
	encoded, err := windows.UTF16FromString(command)
	if err != nil {
		return "", err
	}
	if len(encoded)-1 > 260 {
		return "", fmt.Errorf("Evela's executable path is too long for Windows startup")
	}
	return command, nil
}
