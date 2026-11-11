package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const logFolderWait = 15 * time.Second

type logFolderResult struct {
	snapshot *logSnapshot
	cutoff   time.Time
	err      error
}

type watchedLogFolder struct {
	path          string
	index         logIndex
	last          *logSnapshot
	done          chan logFolderResult
	cancel        context.CancelFunc
	active        bool
	issue         string
	scannedCutoff time.Time
}

// Only the log watcher owns these fields. Each folder has at most one scan in
// flight, even if a disconnected share leaves a Windows file operation blocked.
type multiLogIndex struct {
	cache   *logCache
	folders map[string]*watchedLogFolder
	notify  chan struct{}
	wait    time.Duration
	read    func(context.Context, *logIndex, string, time.Time) (*logSnapshot, error)
}

func (folder *watchedLogFolder) collect() bool {
	if !folder.active {
		return false
	}
	select {
	case result := <-folder.done:
		folder.active = false
		folder.scannedCutoff = result.cutoff
		folder.cancel()
		if result.err != nil {
			folder.issue = result.err.Error()
		} else {
			folder.last, folder.issue = result.snapshot, ""
		}
		return true
	default:
		return false
	}
}

func (index *multiLogIndex) refresh(ctx context.Context, paths []string, cutoff time.Time, progress func(int, int)) (*logSnapshot, error) {
	paths, err := parseLogFolders(strings.Join(paths, "\n"))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if index.folders == nil {
		index.folders = map[string]*watchedLogFolder{}
	}
	if index.notify == nil {
		index.notify = make(chan struct{}, 1)
	}
	keep := map[string]bool{}
	for _, path := range paths {
		keep[logFolderKey(path)] = true
	}
	for key, folder := range index.folders {
		if !keep[key] {
			if folder.cancel != nil {
				folder.cancel()
			}
			delete(index.folders, key)
		}
	}
	var folders []*watchedLogFolder
	awaited := map[*watchedLogFolder]bool{}
	for _, path := range paths {
		key := logFolderKey(path)
		folder := index.folders[key]
		if folder == nil {
			folder = &watchedLogFolder{path: path, done: make(chan logFolderResult, 1), index: logIndex{cache: index.cache}}
			index.folders[key] = folder
		}
		folders = append(folders, folder)
		completed := folder.collect()
		if !folder.active && (!completed || !folder.scannedCutoff.Equal(cutoff)) {
			scanCtx, cancel := context.WithCancel(ctx)
			folder.active, folder.cancel = true, cancel
			awaited[folder] = true
			read := index.read
			notify := index.notify
			go func(folder *watchedLogFolder) {
				var snapshot *logSnapshot
				var err error
				if read != nil {
					snapshot, err = read(scanCtx, &folder.index, folder.path, cutoff)
				} else {
					snapshot, err = folder.index.refresh(scanCtx, folder.path, cutoff, nil)
				}
				folder.done <- logFolderResult{snapshot: snapshot, cutoff: cutoff, err: err}
				select {
				case notify <- struct{}{}:
				default:
				}
			}(folder)
		}
	}
	wait := index.wait
	if wait <= 0 {
		wait = logFolderWait
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
waiting:
	for {
		done := 0
		pending := false
		for _, folder := range folders {
			folder.collect()
			if !folder.active {
				done++
			}
			pending = pending || (folder.active && awaited[folder])
		}
		if progress != nil {
			progress(done, len(folders))
		}
		if !pending {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			break waiting
		case <-index.notify:
		}
	}
	combined := &logSnapshot{SourceFiles: map[string]cachedLog{}}
	for _, folder := range folders {
		folder.collect()
		if folder.active || folder.issue != "" {
			issue := folder.issue
			if folder.active {
				issue = "folder scan is still running"
			}
			if folder.last != nil {
				issue += "; using last successfully loaded data"
			}
			combined.Warnings = append(combined.Warnings, folder.path+": "+issue)
		}
		if folder.last == nil {
			continue
		}
		last := folder.last
		combined.Files += last.Files
		combined.CachedFiles += last.CachedFiles
		combined.ParsedFiles += last.ParsedFiles
		combined.SkippedOld += last.SkippedOld
		combined.SkippedUndated += last.SkippedUndated
		for _, warning := range last.Warnings {
			combined.Warnings = append(combined.Warnings, folder.path+": "+warning)
		}
		for path, cached := range last.SourceFiles {
			// Retained offline data still follows a newly narrowed age limit.
			if !cutoff.IsZero() {
				date, ok := logFilenameDate(filepath.Base(path))
				if !ok {
					combined.SkippedUndated++
					combined.Files--
					continue
				}
				if date.Before(cutoff) {
					combined.SkippedOld++
					combined.Files--
					continue
				}
			}
			combined.SourceFiles[path] = cached
		}
	}
	combined.populateLogs()
	return combined, nil
}

func (index *multiLogIndex) close() {
	for _, folder := range index.folders {
		if folder.cancel != nil {
			folder.cancel()
		}
	}
}

// Cancel old scans before clearing SQLite so they cannot repopulate the cache
// with pre-invalidation results. Detached scans retain only their own folder
// state; newly requested scans start with empty in-memory indexes.
func (index *multiLogIndex) invalidate(ctx context.Context) error {
	index.close()
	index.folders = nil
	if err := index.cache.invalidate(ctx); err != nil {
		if index.cache != nil {
			index.cache.disabled.Store(true)
		}
		return err
	}
	return nil
}

func (snapshot *logSnapshot) warningDetails() string {
	if len(snapshot.Warnings) == 0 {
		return ""
	}
	return fmt.Sprintf(" %d file/folder issue(s):\n%s", len(snapshot.Warnings), strings.Join(snapshot.Warnings, "\n"))
}
