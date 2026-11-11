package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type cachedLog struct {
	Size     int64
	Modified time.Time
	Log      *parsedLog
}

type logIndex struct {
	files map[string]cachedLog
	cache *logCache
}

type logSnapshot struct {
	CachedFiles, ParsedFiles      int
	Logs                          []*parsedLog
	Characters                    []string
	First, Last                   time.Time
	Files, Unassigned, Duplicates int
	SkippedOld, SkippedUndated    int
	Warnings                      []string
	LocationSessions              logLocationHistory
	SourceFiles                   map[string]cachedLog
}

// refresh keeps immutable parsed results for unchanged files. A few workers keep
// the first large import moving without creating a goroutine for every log.
func (index *logIndex) refresh(ctx context.Context, directory string, cutoff time.Time, progress func(int, int)) (*logSnapshot, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("open game logs: %w", err)
	}
	type job struct {
		path string
		info os.FileInfo
	}
	type answer struct {
		path     string
		cached   cachedLog
		err      error
		cacheErr error
		reused   bool
	}
	var jobs []job
	next := make(map[string]cachedLog)
	snapshot := &logSnapshot{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			continue
		}
		// Filter names before stat/open/parse, including cached files. Rebuilding
		// next also releases parsed logs that have aged out or been excluded.
		if !cutoff.IsZero() {
			date, ok := logFilenameDate(entry.Name())
			if !ok {
				snapshot.SkippedUndated++
				continue
			}
			if date.Before(cutoff) {
				snapshot.SkippedOld++
				continue
			}
		}
		path := filepath.Join(directory, entry.Name())
		info, statErr := entry.Info()
		if statErr != nil {
			snapshot.Warnings = append(snapshot.Warnings, entry.Name()+": "+statErr.Error())
			continue
		}
		if cached, ok := index.files[path]; ok && cached.Size == info.Size() && cached.Modified.Equal(info.ModTime()) {
			next[path] = cached
			snapshot.CachedFiles++
		} else {
			jobs = append(jobs, job{path, info})
		}
	}
	snapshot.Files = len(jobs) + len(next)
	work := make(chan job)
	answers := make(chan answer, 4)
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range work {
				parsed, cacheErr := index.cache.load(ctx, task.path, task.info)
				reused := parsed != nil
				cached := cachedLog{task.info.Size(), task.info.ModTime(), parsed}
				var openErr error
				if parsed == nil {
					var file *os.File
					file, openErr = os.Open(task.path)
					if openErr == nil {
						cached, openErr = readLogSnapshot(file)
						file.Close()
					}
				}
				if openErr == nil && !reused && cached.Size >= 0 && ctx.Err() == nil {
					if err := index.cache.store(ctx, task.path, cached); err != nil && cacheErr == nil {
						cacheErr = err
					}
				}
				select {
				case answers <- answer{path: task.path, cached: cached, err: openErr, cacheErr: cacheErr, reused: reused}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, task := range jobs {
			select {
			case work <- task:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(answers) }()
	done := len(next)
	cacheWarned := false
	for answer := range answers {
		if answer.cacheErr != nil && !cacheWarned && ctx.Err() == nil {
			snapshot.Warnings = append(snapshot.Warnings, "SQLite cache unavailable for a log; reading source files: "+answer.cacheErr.Error())
			cacheWarned = true
		}
		if answer.err != nil {
			snapshot.Warnings = append(snapshot.Warnings, filepath.Base(answer.path)+": "+answer.err.Error())
			// Keep the previous successful read if a live file is temporarily locked.
			if previous, ok := index.files[answer.path]; ok {
				next[answer.path] = previous
			}
		} else {
			next[answer.path] = answer.cached
			if answer.reused {
				snapshot.CachedFiles++
			} else {
				snapshot.ParsedFiles++
			}
		}
		done++
		if progress != nil && (done%200 == 0 || done == snapshot.Files) {
			progress(done, snapshot.Files)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index.files = next
	snapshot.SourceFiles = next
	snapshot.populateLogs()
	return snapshot, nil
}

func (snapshot *logSnapshot) populateLogs() {
	characters := make(map[string]bool)
	seen := make(map[[32]byte]bool)
	// Deterministic source ordering also makes duplicate selection stable when
	// the same export is present in a local folder and on a network share.
	paths := make([]string, 0, len(snapshot.SourceFiles))
	for path := range snapshot.SourceFiles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		cached := snapshot.SourceFiles[path]
		log := cached.Log
		if log.Character == "" {
			snapshot.Unassigned++
			continue
		}
		characters[log.Character] = true
		if seen[log.Hash] {
			snapshot.Duplicates++
			continue
		}
		seen[log.Hash] = true
		snapshot.Logs = append(snapshot.Logs, log)
		first, last := log.ActivityBounds()
		if !first.IsZero() && (snapshot.First.IsZero() || first.Before(snapshot.First)) {
			snapshot.First = first
		}
		if !last.IsZero() && (snapshot.Last.IsZero() || last.After(snapshot.Last)) {
			snapshot.Last = last
		}
	}
	for character := range characters {
		snapshot.Characters = append(snapshot.Characters, character)
	}
	sort.Strings(snapshot.Characters)
	snapshot.LocationSessions = snapshot.locationSessions()
}
