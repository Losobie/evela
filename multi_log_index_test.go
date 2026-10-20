package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeFolderLog(t *testing.T, folder, name, pilot string, amount int) {
	t.Helper()
	content := strings.ReplaceAll(sampleLog, "Test Pilot", pilot)
	if amount != 0 {
		content += fmt.Sprintf("[ 2026.10.17 02:01:03 ] (combat) %d to Target - Hits\n", amount)
	}
	if err := os.WriteFile(filepath.Join(folder, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMultipleLogFoldersMergeCacheDuplicatesAndRemove(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	const name = "20261017_020000_123.txt"
	writeFolderLog(t, local, name, "Local Pilot", 0)
	writeFolderLog(t, remote, name, "Remote Pilot", 0)
	writeFolderLog(t, remote, "20261017_duplicate_123.txt", "Local Pilot", 0)
	index := &multiLogIndex{}
	defer index.close()
	refresh := func(paths ...string) *logSnapshot {
		t.Helper()
		snapshot, err := index.refresh(context.Background(), paths, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	snapshot := refresh(local, remote, local)
	if snapshot.Files != 3 || snapshot.Duplicates != 1 || !slices.Equal(snapshot.Characters, []string{"Local Pilot", "Remote Pilot"}) || len(snapshot.Logs) != 2 {
		t.Fatalf("folders were not combined/deduplicated: %+v", snapshot)
	}
	localLog := snapshot.SourceFiles[filepath.Join(local, name)].Log
	remoteLog := snapshot.SourceFiles[filepath.Join(remote, name)].Log
	if localLog == remoteLog {
		t.Fatal("same filename in different folders collided")
	}
	snapshot = refresh(local, remote)
	if snapshot.SourceFiles[filepath.Join(local, name)].Log != localLog || snapshot.SourceFiles[filepath.Join(remote, name)].Log != remoteLog {
		t.Fatal("unchanged sources were reparsed")
	}
	writeFolderLog(t, remote, name, "Remote Pilot", 1)
	snapshot = refresh(local, remote)
	if snapshot.SourceFiles[filepath.Join(local, name)].Log != localLog || snapshot.SourceFiles[filepath.Join(remote, name)].Log == remoteLog || snapshot.Last.Unix() != remoteLog.Points[0].Second+3 {
		t.Fatal("remote change did not update independently")
	}
	snapshot = refresh(local)
	if snapshot.Files != 1 || len(snapshot.Characters) != 1 || snapshot.Characters[0] != "Local Pilot" || len(index.folders) != 1 {
		t.Fatal("removed folder retained its damage/cache")
	}
	if err := os.Remove(filepath.Join(local, name)); err != nil {
		t.Fatal(err)
	}
	snapshot = refresh(local)
	if len(snapshot.Logs) != 0 || snapshot.Files != 0 {
		t.Fatal("successfully scanned deleted file remained cached")
	}
}

func TestUnavailableLogFolderRetainsDataFiltersByAgeAndRecovers(t *testing.T) {
	local := t.TempDir()
	root := t.TempDir()
	remote := filepath.Join(root, "share")
	if err := os.Mkdir(remote, 0700); err != nil {
		t.Fatal(err)
	}
	writeFolderLog(t, local, "20261017_020000_123.txt", "Local Pilot", 0)
	writeFolderLog(t, remote, "20261016_020000_456.txt", "Remote Pilot", 0)
	index := &multiLogIndex{}
	defer index.close()
	refresh := func(cutoff time.Time) *logSnapshot {
		t.Helper()
		snapshot, err := index.refresh(context.Background(), []string{local, remote}, cutoff, nil)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	refresh(time.Time{})
	if err := os.Rename(remote, filepath.Join(root, "offline")); err != nil {
		t.Fatal(err)
	}
	writeFolderLog(t, local, "20261017_020000_123.txt", "Local Pilot", 1)
	snapshot := refresh(time.Time{})
	if len(snapshot.Characters) != 2 || len(snapshot.Warnings) != 1 || !strings.Contains(snapshot.Warnings[0], remote) || !strings.Contains(snapshot.Warnings[0], "last successfully loaded") {
		t.Fatalf("outage removed data or hid diagnostics: %+v", snapshot)
	}
	if len(snapshot.SourceFiles[filepath.Join(local, "20261017_020000_123.txt")].Log.Points) != 3 {
		t.Fatal("offline remote stopped local updates")
	}
	snapshot = refresh(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	if len(snapshot.Characters) != 1 || snapshot.Characters[0] != "Local Pilot" || snapshot.SkippedOld != 1 {
		t.Fatal("retained offline data ignored a narrower age limit")
	}
	if err := os.Rename(filepath.Join(root, "offline"), remote); err != nil {
		t.Fatal(err)
	}
	writeFolderLog(t, remote, "20261017_020000_456.txt", "Recovered Pilot", 0)
	snapshot = refresh(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	if len(snapshot.Warnings) != 0 || !slices.Contains(snapshot.Characters, "Recovered Pilot") {
		t.Fatal("remote share was not retried/recovered")
	}
	missing := filepath.Join(root, "missing")
	snapshot, err := index.refresh(context.Background(), []string{local, missing}, time.Time{}, nil)
	if err != nil || len(snapshot.Characters) != 1 || len(snapshot.Warnings) != 1 {
		t.Fatalf("a missing new share stopped valid local data: %+v %v", snapshot, err)
	}
}

func TestSlowLogFolderDoesNotBlockOthersOrStartDuplicateScans(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	writeFolderLog(t, local, "local.txt", "Local Pilot", 0)
	writeFolderLog(t, remote, "remote.txt", "Remote Pilot", 0)
	blocked := make(chan struct{})
	var remoteScans atomic.Int32
	index := &multiLogIndex{wait: 50 * time.Millisecond}
	index.read = func(ctx context.Context, index *logIndex, path string, cutoff time.Time) (*logSnapshot, error) {
		if path == remote {
			remoteScans.Add(1)
			<-blocked
		}
		return index.refresh(ctx, path, cutoff, nil)
	}
	defer index.close()
	refresh := func() *logSnapshot {
		t.Helper()
		snapshot, err := index.refresh(context.Background(), []string{local, remote}, time.Time{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	snapshot := refresh()
	if !slices.Equal(snapshot.Characters, []string{"Local Pilot"}) || len(snapshot.Warnings) != 1 {
		t.Fatal("blocked share prevented local results")
	}
	writeFolderLog(t, local, "local.txt", "Local Pilot", 1)
	snapshot = refresh()
	if len(snapshot.Logs[0].Points) != 3 || remoteScans.Load() != 1 {
		t.Fatal("blocked share prevented another local refresh or started duplicate blocked scans")
	}
	close(blocked)
	// Wait for the simulated network operation to return through its real reader.
	select {
	case result := <-index.folders[logFolderKey(remote)].done:
		index.folders[logFolderKey(remote)].done <- result
	case <-time.After(5 * time.Second):
		t.Fatal("remote scan did not complete")
	}
	snapshot = refresh()
	if len(snapshot.Characters) != 2 || len(snapshot.Warnings) != 0 || remoteScans.Load() != 1 {
		t.Fatal("completed background scan was not included")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := index.refresh(ctx, []string{local}, time.Time{}, nil); err != context.Canceled {
		t.Fatal("cancelled refresh was not stopped")
	}
}

func TestLogFolderNormalizationSupportsUNCAndAvoidsDuplicatePaths(t *testing.T) {
	local := t.TempDir()
	input := "  \"" + local + "\" \n" + filepath.Join(local, ".") + "\n\n"
	if runtime.GOOS == "windows" {
		input += strings.ToUpper(local) + "\n"
	}
	folders, err := parseLogFolders(input)
	if err != nil || !slices.Equal(folders, []string{local}) {
		t.Fatalf("duplicate/quoted paths not normalized: %+v %v", folders, err)
	}
	if runtime.GOOS == "windows" {
		unc := `\\fleet-pc\eve logs\gamelogs`
		folders, err = parseLogFolders(local + "\r\n" + unc)
		if err != nil || !slices.Equal(folders, []string{local, unc}) {
			t.Fatalf("UNC path mangled: %+v %v", folders, err)
		}
	}
	if _, err := parseLogFolders("\n \t"); err == nil {
		t.Fatal("empty folder list accepted")
	}
}
