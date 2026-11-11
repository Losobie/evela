package main

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestForceReprocessClearsMemoryAndDatabase(t *testing.T) {
	folder := t.TempDir()
	writeCacheLog(t, folder, "20261111_log.txt", sampleLog)
	csvPath := filepath.Join(folder, "StatLog_Taredi Kisi Khagah_2026-11-10.csv")
	if err := os.WriteFile(csvPath, []byte("timestamp,stat\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	index := &multiLogIndex{cache: cache}
	defer index.close()
	refresh := func() *logSnapshot {
		t.Helper()
		snapshot, err := index.refresh(context.Background(), []string{folder}, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	refresh()
	if refresh().CachedFiles != 1 {
		t.Fatal("fixture did not populate caches")
	}
	if err := index.invalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"log_files", "parsed_logs"} {
		var count int
		if err := cache.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cache table %s not cleared: %d/%v", table, count, err)
		}
	}
	snapshot := refresh()
	if snapshot.ParsedFiles != 1 || snapshot.CachedFiles != 0 || len(snapshot.Logs) != 1 {
		t.Fatal("unchanged source was not reprocessed")
	}
	if refresh().CachedFiles != 1 {
		t.Fatal("cache did not resume after reprocessing")
	}
	files, err := os.ReadDir(folder)
	if err != nil || len(files) != 2 || files[0].Name() != "20261111_log.txt" {
		t.Fatal("reprocessing wrote artifacts into the log folder")
	}
	if data, err := os.ReadFile(csvPath); err != nil || string(data) != "timestamp,stat\n" {
		t.Fatal("reprocessing modified an unrelated CSV")
	}
}

func TestForceReprocessFailureStillBypassesStaleCache(t *testing.T) {
	folder := t.TempDir()
	writeCacheLog(t, folder, "20261111_log.txt", sampleLog)
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	index := &multiLogIndex{cache: cache}
	defer index.close()
	if _, err := index.refresh(context.Background(), []string{folder}, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.db.Exec(`CREATE TRIGGER fail_clear BEFORE DELETE ON parsed_logs BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := index.invalidate(context.Background()); err == nil || !cache.disabled.Load() {
		t.Fatal("failed invalidation did not disable cache reuse")
	}
	snapshot, err := index.refresh(context.Background(), []string{folder}, time.Time{}, nil)
	if err != nil || snapshot.ParsedFiles != 1 || snapshot.CachedFiles != 0 {
		t.Fatalf("cache failure prevented forced source processing: %+v/%v", snapshot, err)
	}
	if _, err := cache.db.Exec("DROP TRIGGER fail_clear"); err != nil {
		t.Fatal(err)
	}
	if err := index.invalidate(context.Background()); err != nil || cache.disabled.Load() {
		t.Fatal("cache could not recover on the next successful invalidation")
	}
}

func TestForceReprocessDetachesAndCancelsOldScans(t *testing.T) {
	folder := t.TempDir()
	writeCacheLog(t, folder, "20261111_log.txt", sampleLog)
	release, stopped := make(chan struct{}), make(chan struct{})
	defer close(release)
	var scans atomic.Int32
	index := &multiLogIndex{wait: time.Millisecond}
	defer index.close()
	index.read = func(ctx context.Context, fileIndex *logIndex, path string, cutoff time.Time) (*logSnapshot, error) {
		if scans.Add(1) == 1 {
			<-ctx.Done()
			close(stopped)
			<-release
		}
		return fileIndex.refresh(ctx, path, cutoff, nil)
	}
	if _, err := index.refresh(context.Background(), []string{folder}, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := index.invalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("old scan was not cancelled")
	}
	index.wait = time.Second
	snapshot, err := index.refresh(context.Background(), []string{folder}, time.Time{}, nil)
	if err != nil || snapshot.ParsedFiles != 1 || len(snapshot.Logs) != 1 || scans.Load() != 2 {
		t.Fatalf("new read reused old folder state: %+v/%v", snapshot, err)
	}
}
