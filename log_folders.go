package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
)

const logFoldersPreference = "logs.folders.v1"

func logFolderKey(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(path))
	}
	return filepath.Clean(path)
}

func parseLogFolders(text string) ([]string, error) {
	var folders []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		path := strings.Trim(strings.TrimSpace(line), `"`)
		if path == "" {
			continue
		}
		if strings.ContainsRune(path, 0) {
			return nil, fmt.Errorf("folder paths cannot contain a null character")
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("invalid log folder: %w", err)
		}
		key := logFolderKey(path)
		if !seen[key] {
			folders = append(folders, path)
			seen[key] = true
		}
	}
	if len(folders) == 0 {
		return nil, fmt.Errorf("add at least one EVE logs/gamelogs folder")
	}
	return folders, nil
}

func savedLogFolders() []string {
	var folders []string
	if json.Unmarshal([]byte(fyne.CurrentApp().Preferences().String(logFoldersPreference)), &folders) == nil {
		if normalized, err := parseLogFolders(strings.Join(folders, "\n")); err == nil {
			return normalized
		}
	}
	return []string{defaultLogDirectory()}
}

func saveLogFolders(folders []string) {
	data, _ := json.Marshal(folders)
	fyne.CurrentApp().Preferences().SetString(logFoldersPreference, string(data))
}
