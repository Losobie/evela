package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/unicode"
)

const sampleLog = "\ufeff------------------------------------------------------------\r\n" +
	"  Gamelog\r\n  Listener: Test Pilot\r\n  Session Started: 2026.10.17 02:00:00\r\n" +
	"[ 2026.10.17 02:01:00 ] (combat) <color=0xff00ffff><b>1,200</b> <font size=10>to</font> <b>Enemy</b> - Laser - Hits\r\n" +
	"[ 2026.10.17 02:01:00 ] (combat) 50.5 to Enemy - Drone - Hits\r\n" +
	"[ 2026.10.17 02:01:01 ] (combat) <b>612</b> <font size=10>from</font> Enemy - Laser - Glances Off\r\n" +
	"[ 2026.10.17 02:01:02 ] (combat) 241 GJ energy neutralized Enemy - Neutralizer\r\n" +
	"[ 2026.10.17 02:01:02 ] (combat) 100 remote armor repaired to Enemy\r\n" +
	"[ 2026.10.17 02:01:02 ] (combat) Enemy misses you completely - Laser\r\n" +
	"[ 2026.10.17 02:01:02 ] (notify) 999 to Enemy - Not combat\r\n" +
	"[ 2026.99.17 02:01:02 ] (combat) 999 to Enemy - Bad time\r\n" +
	"[ 2026.10.17 02:01:02 ] (combat) 1.2.3 to Enemy - Bad number\r\n"

func TestParseLog(t *testing.T) {
	for _, encoding := range []string{"utf8", "utf16le", "utf16be"} {
		t.Run(encoding, func(t *testing.T) {
			data := []byte(sampleLog)
			var err error
			if encoding == "utf16le" {
				data, err = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(data)
			}
			if encoding == "utf16be" {
				data, err = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewEncoder().Bytes(data)
			}
			if err != nil {
				t.Fatal(err)
			}
			log, err := parseLog(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if log.Character != "Test Pilot" || len(log.Points) != 2 {
				t.Fatalf("unexpected result: %+v", log)
			}
			if len(log.EWar) != 1 || log.EWar[0].Amount != 241 {
				t.Fatalf("missing encoded E-War record: %+v", log.EWar)
			}
			if point := log.Points[0]; point.Outgoing != 1250.5 || point.OutgoingHits != 2 || point.Incoming != 0 {
				t.Fatalf("outgoing: %+v", point)
			}
			if point := log.Points[1]; point.Incoming != 612 || point.IncomingHits != 1 || point.Outgoing != 0 {
				t.Fatalf("incoming: %+v", point)
			}
		})
	}
}

func TestIndexRefreshAndDuplicateExports(t *testing.T) {
	directory := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("one.txt", sampleLog)
	write("copy.TXT", sampleLog)
	write("ignored.md", sampleLog)
	write("unassigned.txt", "Gamelog\n")
	index := &logIndex{}
	snapshot, err := index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Files != 3 || snapshot.Duplicates != 1 || snapshot.Unassigned != 1 || len(snapshot.Logs) != 1 || len(snapshot.Characters) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	original := index.files[filepath.Join(directory, "one.txt")].Log
	if _, err := index.refresh(context.Background(), directory, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if index.files[filepath.Join(directory, "one.txt")].Log != original {
		t.Fatal("unchanged file was reparsed")
	}
	write("one.txt", sampleLog+"[ 2026.10.17 02:01:03 ] (combat) 10 to Target - Hits\n")
	if err := os.Remove(filepath.Join(directory, "copy.TXT")); err != nil {
		t.Fatal(err)
	}
	write("two.txt", strings.ReplaceAll(sampleLog, "Test Pilot", "Another Pilot"))
	snapshot, err = index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Duplicates != 0 || len(snapshot.Characters) != 2 || snapshot.Last.Format("15:04:05") != "02:01:03" {
		t.Fatalf("refresh did not reflect changed files: %+v", snapshot)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := index.refresh(ctx, directory, time.Time{}, nil); err == nil {
		t.Fatal("cancelled scan succeeded")
	}
}

func TestBuildViewBoundariesAndPartialBucket(t *testing.T) {
	start := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	log := &parsedLog{Character: "A", Points: []damagePoint{
		{Second: start.Unix() - 1, Outgoing: 1000},
		{Second: start.Unix(), Outgoing: 100, OutgoingHits: 1},
		{Second: start.Unix() + 4, Incoming: 60, IncomingHits: 1},
		{Second: start.Unix() + 5, Outgoing: 20, OutgoingHits: 1},
		{Second: start.Unix() + 6, Outgoing: 500},
	}}
	snapshot := &logSnapshot{Characters: []string{"A", "B"}, Logs: []*parsedLog{log}}
	view := buildView(snapshot, start, start.Add(6*time.Second), 5*time.Second)
	a := view.Series[0]
	if len(a.Outgoing) != 2 || a.Outgoing[0] != 20 || a.Outgoing[1] != 20 || a.Incoming[0] != 12 || a.TotalOutgoing != 120 || a.TotalIncoming != 60 || a.OutgoingHits != 2 {
		t.Fatalf("incorrect rates/bounds: %+v", a)
	}
	if view.Series[1].TotalOutgoing != 0 {
		t.Fatal("idle character acquired damage")
	}
	long := buildView(snapshot, start, start.Add(400*24*time.Hour), time.Second)
	if len(long.Series[0].Outgoing) > 1200 {
		t.Fatal("large range exceeds bucket bound")
	}
}

func TestRangesUseLatestLogAndUTC(t *testing.T) {
	first := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 10, 17, 2, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{First: first, Last: last}
	var start, end time.Time
	for _, preset := range []struct {
		name     string
		duration time.Duration
	}{
		{"Last 10 minutes", 10 * time.Minute},
		{"Last 30 minutes", 30 * time.Minute},
		{"Last hour", time.Hour},
	} {
		start, end = presetRange(preset.name, snapshot)
		if !end.Equal(last.Add(time.Second)) || end.Sub(start) != preset.duration {
			t.Fatalf("%s should cover %s through the latest log; got %s — %s", preset.name, preset.duration, start, end)
		}
	}
	start, end = presetRange("All history", snapshot)
	if !start.Equal(first) || !end.After(last) {
		t.Fatal("all history does not include boundary events")
	}
	if _, _, err := parseRange("2026-10-17", "2026-10-18 02:00"); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"bad", "2026-10-18"}, {"2026-10-18", "2026-10-17"}, {"2026-10-17", "2026-10-17"}} {
		if _, _, err := parseRange(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted invalid range: %v", pair)
		}
	}
}

// Opt-in integration check against real, read-only logs. No game data is written.
func TestLocalLogs(t *testing.T) {
	directory := os.Getenv("EVELA_TEST_LOGS")
	if directory == "" {
		t.Skip("set EVELA_TEST_LOGS to verify a real EVE log directory")
	}
	index := &logIndex{}
	started := time.Now()
	snapshot, err := index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Characters) == 0 || snapshot.Last.IsZero() {
		t.Fatal("no characters or damage found in real logs")
	}
	start, end := presetRange("Last 24 hours", snapshot)
	view := buildView(snapshot, start, end, 0)
	var incoming, outgoing float64
	var active int
	for _, series := range view.Series {
		incoming += series.TotalIncoming
		outgoing += series.TotalOutgoing
		if series.TotalIncoming+series.TotalOutgoing > 0 {
			active++
		}
	}
	t.Logf("%d files; %d characters; %d active latest day; %d duplicates; %d unassigned; %d warnings; latest=%s; incoming=%.0f outgoing=%.0f; indexed in %s", snapshot.Files, len(snapshot.Characters), active, snapshot.Duplicates, snapshot.Unassigned, len(snapshot.Warnings), snapshot.Last.Format(time.RFC3339), incoming, outgoing, time.Since(started))
	if len(snapshot.Warnings) > 0 {
		t.Log(fmt.Sprint(snapshot.Warnings[:min(3, len(snapshot.Warnings))]))
	}
	started = time.Now()
	if _, err = index.refresh(context.Background(), directory, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("cached refresh: %s", time.Since(started))
}
