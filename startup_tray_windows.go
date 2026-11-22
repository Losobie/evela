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

func windowsStartupControls() (fyne.CanvasObject, func() error) {
	enabled, minimized, err := readWindowsStartup()
	if err != nil {
		reportRuntimeError("Read Windows startup settings", err)
		message := widget.NewLabel("Windows startup settings are unavailable.")
		return message, nil
	}
	startMinimized := widget.NewCheck("Start minimized to system tray", nil)
	startMinimized.SetChecked(minimized)
	startAtSignIn := widget.NewCheck("Start when I sign in to Windows", func(checked bool) {
		if checked {
			startMinimized.Enable()
		} else {
			startMinimized.Disable()
		}
	})
	startAtSignIn.SetChecked(enabled)
	if !enabled {
		startMinimized.Disable()
	}
	return container.NewVBox(startAtSignIn, startMinimized), func() error {
		if !enabled && !startAtSignIn.Checked {
			return nil
		}
		return saveWindowsStartup(startAtSignIn.Checked, startMinimized.Checked)
	}
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
