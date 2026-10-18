//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func defaultLogDirectory() string {
	documents, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		home, _ := os.UserHomeDir()
		documents = filepath.Join(home, "Documents")
	}
	return filepath.Join(documents, "EVE", "logs", "gamelogs")
}
