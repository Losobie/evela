//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func defaultLogDirectory() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", "EVE", "logs", "gamelogs")
}
