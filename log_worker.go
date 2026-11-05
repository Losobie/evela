package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func (ui *analyzerUI) requestLoad() {
	folders, err := parseLogFolders(ui.path.Text)
	if err != nil {
		dialog.ShowError(err, ui.window)
		return
	}
	select {
	case <-ui.loadRequests:
	default:
	}
	ui.loadRequests <- folders
}

// The worker owns the file cache. All widgets and view state stay on Fyne's UI thread.
func (ui *analyzerUI) watch() {
	cachePath, cacheErr := defaultLogCachePath()
	var cache *logCache
	if cacheErr == nil {
		cache, cacheErr = openLogCache(cachePath, logParserVersion)
	}
	defer cache.close()
	index := &multiLogIndex{cache: cache}
	defer index.close()
	var directories []string
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ui.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	for {
		select {
		case <-ui.stop:
			return
		case directories = <-ui.loadRequests:
		case <-ticker.C:
			enabled := false
			fyne.DoAndWait(func() { enabled = ui.auto.Checked })
			if !enabled || len(directories) == 0 {
				continue
			}
		}
		var lookbackDays int
		fyne.DoAndWait(func() {
			lookbackDays = ui.logLookbackDays
			ui.refresh.Disable()
			ui.progress.SetValue(0)
			ui.progress.Show()
			ui.status.SetText("Reading logs… unchanged files use the local cache.")
		})
		snapshot, err := index.refresh(ctx, directories, logLoadingCutoff(time.Now(), lookbackDays), func(done, total int) {
			fyne.Do(func() {
				ui.progress.SetValue(float64(done) / float64(total))
				ui.status.SetText(fmt.Sprintf("Refreshed %d / %d log folders…", done, total))
			})
		})
		if ctx.Err() != nil {
			return
		}
		fyne.DoAndWait(func() {
			ui.refresh.Enable()
			ui.progress.Hide()
			if err != nil {
				ui.status.SetText("Could not load folders: " + err.Error())
				return
			}
			currentFolders, _ := parseLogFolders(ui.path.Text)
			if lookbackDays != ui.logLookbackDays || !slices.Equal(directories, currentFolders) {
				// Saving settings queues a new load. Do not display a completed
				// scan from the previous limit while that request is pending.
				return
			}
			if cacheErr != nil {
				snapshot.Warnings = append(snapshot.Warnings, "SQLite cache unavailable; using source logs: "+cacheErr.Error())
			}
			ui.snapshot = snapshot
			ui.rememberLogLocations(snapshot.locationSessions())
			directoryKey := strings.Join(directories, "\n")
			if ui.logDirectory != directoryKey {
				ui.clearGraphRangeHistory()
				ui.logDirectory = directoryKey
			}
			for _, name := range snapshot.Characters {
				if _, known := ui.selected[name]; !known {
					ui.selected[name] = ui.damageFilterSelected(filterCharacters, name)
				}
			}
			ui.updateRange()
			ui.directoryInfo.SetText(fmt.Sprintf("%d log folders · %d log files loaded · %d too old · %d without filename dates · %d characters · %d duplicate exports skipped · %d files without a Listener header", len(directories), snapshot.Files, snapshot.SkippedOld, snapshot.SkippedUndated, len(snapshot.Characters), snapshot.Duplicates, snapshot.Unassigned))
			status := "Updated " + time.Now().Format("15:04:05") + fmt.Sprintf(" · %d cached / %d processed logs · Logs stay on this computer.", snapshot.CachedFiles, snapshot.ParsedFiles)
			if snapshot.Files == 0 {
				if snapshot.SkippedOld+snapshot.SkippedUndated > 0 {
					status += " No logs matched the loading limit. Change Settings → Log loading to include older files."
				} else {
					status += " No .txt game logs found in the configured folders."
				}
			} else if snapshot.Last.IsZero() {
				status += " No supported English combat or mining messages found."
			} else {
				status += " Latest activity: " + snapshot.Last.Format("2006-01-02 15:04:05") + " EVE time."
			}
			status += snapshot.warningDetails()
			ui.status.SetText(status)
		})
	}
}
