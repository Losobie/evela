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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/text/encoding/unicode"
)

const movementLog = "Listener: Pilot A\nSession Started: 2026.10.19 02:00:00\n" +
	"[ 2026.10.19 02:00:01 ] (combat) 100 to Target - Hits\n" +
	"[ 2026.10.19 02:00:10 ] (None) Jumping from <b>Mamet</b> to <a href=showinfo:5//30000001>Hoshoun</a>\n" +
	"[ 2026.10.19 02:00:11 ] (combat) 200 to Target - Hits\n" +
	"[ 2026.10.19 02:00:20 ] (notify) A Conduit Field activated by Clento Loso jumps you to Mai.\n" +
	"[ 2026.10.19 02:00:21 ] (combat) 300 to Target - Hits\n" +
	"[ 2026.10.19 02:00:30 ] (notify) Activating a Conduit Field jumps you to Camal, bringing along 22 passengers.\n" +
	"[ 2026.10.19 02:10:00 ] (combat) 400 to Target - Hits\n"

func TestParseLogLocationsMessagesAndEncodings(t *testing.T) {
	for _, encoding := range []string{"utf8", "utf16le", "utf16be"} {
		t.Run(encoding, func(t *testing.T) {
			data := []byte(movementLog)
			if encoding == "utf16le" {
				data, _ = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(data)
			}
			if encoding == "utf16be" {
				data, _ = unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewEncoder().Bytes(data)
			}
			log, err := parseLog(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if len(log.LocationEvents) != 3 || len(log.Points) != 4 {
				t.Fatalf("unexpected parsed locations/damage: %+v", log)
			}
			for i, want := range []string{"Hoshoun", "Mai", "Camal"} {
				if log.LocationEvents[i].To != want {
					t.Fatalf("destination %d: %+v", i, log.LocationEvents[i])
				}
			}
			if log.LocationEvents[0].From != "Mamet" || log.SessionStarted.Format(logTimeLayout) != "2026.10.19 02:00:00" {
				t.Fatal("missing origin or session time")
			}
		})
	}
	for _, tc := range []struct {
		message, to string
		known       bool
	}{
		{"Jumping from Mamet to Hoshoun.", "Hoshoun", true},
		{"Jumping from New Caldari to J123456", "J123456", true},
		{"Activating a Conduit Field jumps you to Camal, bringing along 1 passenger.", "Camal", true},
		{"A Conduit Field activated by Pilot jumps you to New Caldari.", "New Caldari", true},
		{"Jumping from Mamet to ", "", false},
		{"Pilot says: Jumping from Mamet to Hoshoun", "", false},
		{"A Conduit Field activated by Pilot jumps you to .", "", false},
	} {
		event, ok := parseLogLocation(tc.message, 123)
		if ok != tc.known || ok && event.To != tc.to {
			t.Fatalf("%q parsed as %+v, %v", tc.message, event, ok)
		}
	}
	log, err := parseLog(strings.NewReader("Listener: Pilot\n[ 2026.10.19 02:00:10 ] (chat) Jumping from Mamet to Hoshoun\n"))
	if err != nil || len(log.LocationEvents) != 0 {
		t.Fatal("chat text was treated as movement")
	}
}

func TestLogLocationsOverrideCachedESIThroughoutSession(t *testing.T) {
	log, err := parseLog(strings.NewReader(movementLog))
	if err != nil {
		t.Fatal(err)
	}
	start := log.SessionStarted
	history := (&logSnapshot{Logs: []*parsedLog{log}}).locationSessions()
	esi := locationHistory{1: {{CharacterID: 1, CharacterName: "Pilot A", SystemID: 99, SystemName: "Cached system", FirstSeen: start.Add(-time.Second), LastSeen: start.Add(time.Hour)}}}
	resolver := newLocationResolver(esi, history)
	for _, tc := range []struct {
		seconds int
		name    string
	}{
		{-1, "Cached system"}, {0, "Mamet"}, {9, "Mamet"}, {10, "Hoshoun"}, {19, "Hoshoun"},
		{20, "Mai"}, {29, "Mai"}, {30, "Camal"}, {600, "Camal"}, {645, "Camal"}, {646, "Cached system"},
	} {
		location, ok := resolver.atName(" pILOT a ", start.Add(time.Duration(tc.seconds)*time.Second))
		if !ok || location.SystemName != tc.name {
			t.Fatalf("at +%d: %+v, known=%v", tc.seconds, location, ok)
		}
	}
	// A newer, conflicting ESI poll must not override the explicit logged jump.
	esi[1] = append(esi[1], locationPeriod{CharacterID: 1, CharacterName: "Pilot A", SystemID: 99, SystemName: "Cached system", FirstSeen: start.Add(25 * time.Second), LastSeen: start.Add(time.Hour)})
	resolver = newLocationResolver(esi, history)
	if p, _ := resolver.atName("Pilot A", start.Add(35*time.Second)); p.SystemName != "Camal" {
		t.Fatal("new cached ESI poll overrode a logged movement")
	}
	if _, ok := newLocationResolver(nil, history).atName("Pilot B", start.Add(time.Second)); ok {
		t.Fatal("log location leaked between characters")
	}
	if _, ok := newLocationResolver(nil, history).atName("Pilot A", start.Add(-time.Second)); ok {
		t.Fatal("origin extrapolated before session start")
	}
	// A conduit destination cannot establish the system before that jump.
	conduit := logLocationHistory{"conduit": {CharacterName: "Pilot", Start: start.Unix(), End: start.Unix() + 100, Events: []logLocationEvent{{Second: start.Unix() + 50, To: "Mai"}}}}
	if _, ok := newLocationResolver(nil, conduit).atName("Pilot", start.Add(49*time.Second)); ok {
		t.Fatal("conduit destination applied backwards")
	}
}

func TestLogLocationIndexRefreshMergeAndPersistence(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "session.txt")
	if err := os.WriteFile(path, []byte(movementLog), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "copy.txt"), []byte(movementLog), 0600); err != nil {
		t.Fatal(err)
	}
	index := &logIndex{}
	snapshot, err := index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	history := snapshot.locationSessions()
	if len(history) != 1 || snapshot.Duplicates != 1 {
		t.Fatal("duplicate exports duplicated location sessions")
	}
	if history.merge(snapshot.locationSessions()) {
		t.Fatal("unchanged refresh rewrote location history")
	}
	updated := movementLog + "[ 2026.10.19 02:11:00 ] (notify) Jumping from Camal to Mamet\n[ 2026.10.19 02:11:01 ] (combat) 500 to Target - Hits\n"
	if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err = index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !history.merge(snapshot.locationSessions()) || len(history) != 1 {
		t.Fatal("growing log failed to extend existing session")
	}
	for _, session := range history {
		if len(session.Events) != 4 {
			t.Fatal("overlapping exports duplicated jumps")
		}
	}
	service := newESIService(filepath.Join(t.TempDir(), "esi.dat"))
	service.config.LogLocations = history
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	loaded := newESIService(service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 19, 2, 11, 1, 0, time.UTC)
	if location, ok := newLocationResolver(nil, loaded.config.LogLocations).atName("Pilot A", at); !ok || location.SystemName != "Mamet" || len(loaded.config.Accounts) != 0 {
		t.Fatal("log location history did not survive restart without an ESI account")
	}
	copy := history.clone()
	for key, session := range copy {
		session.Events[0].To = "Changed"
		copy[key] = session
	}
	for _, session := range history {
		if session.Events[0].To != "Hoshoun" {
			t.Fatal("history clone shared mutable events")
		}
	}
}

func TestLogLocationsFilterWithoutESIAndKeepSelectionsWhenIDResolves(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	log, err := parseLog(strings.NewReader(movementLog))
	if err != nil {
		t.Fatal(err)
	}
	ui.snapshot = &logSnapshot{Logs: []*parsedLog{log}, Characters: []string{"Pilot A"}}
	ui.applyGraphRange(start, start.Add(11*time.Minute))
	ui.setAllDamageFilterOptions(filterSystem, false)
	ui.setDamageFilterOption(filterSystem, "system-name:camal", true)
	if filteredTotal(ui) != 400 {
		t.Fatal("log-only system did not filter damage")
	}
	// Add ESI metadata from another character, including a conflicting cached
	// location for Pilot A covering all hits. The ID must unify with log names.
	ui.locationHistory = locationHistory{1: {{CharacterID: 1, CharacterName: "Pilot A", SystemID: 100, SystemName: "Mamet", FirstSeen: start, LastSeen: start.Add(time.Hour)}},
		2: {{CharacterID: 2, CharacterName: "Pilot B", SystemID: 200, SystemName: "Camal", FirstSeen: start, LastSeen: start.Add(time.Hour)}}}
	ui.rebuildView()
	if filteredTotal(ui) != 400 || !ui.damageFilterSelected(filterSystem, "system:200") {
		t.Fatal("system ID resolution lost name-only selection or allowed cached ESI to override logs")
	}
	count := 0
	for _, option := range ui.filterOptions[filterSystem] {
		if option.Label == "Camal" {
			count++
			if option.Key != "system:200" {
				t.Fatal("log/ESI systems were not unified")
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicate options for same log/ESI system")
	}
	ui.graphInvulnerabilityMarkers = true
	log.StructureEvents = []structureEvent{{Second: start.Unix() + 11, Target: "Hoshoun target"}, {Second: start.Unix() + 600, Target: "Camal target"}}
	ui.rebuildView()
	if len(ui.outgoing.data.Markers) != 1 || ui.outgoing.data.Markers[0].Label != "Camal target" {
		t.Fatal("markers ignored log-based system filtering")
	}
	other := fyne.CurrentApp().NewWindow("Restored log systems")
	defer other.Close()
	restored := newAnalyzerUI(other)
	restored.snapshot, restored.locationHistory = ui.snapshot, ui.locationHistory
	restored.rebuildView()
	if filteredTotal(restored) != 400 {
		t.Fatal("migrated system selection was not persisted")
	}
	ui.rememberLogLocations(ui.snapshot.locationSessions())
	ui.rememberLogLocations(ui.snapshot.locationSessions())
	select {
	case update := <-ui.logLocationUpdates:
		if len(update) != 1 {
			t.Fatal("missing queued history")
		}
	default:
		t.Fatal("log history was not queued for persistence")
	}
	ui.snapshot = &logSnapshot{}
	if location, ok := ui.resolvedLocations().atName("Pilot A", start.Add(600*time.Second)); !ok || location.SystemName != "Camal" {
		t.Fatal("removing logs from loaded window discarded historical locations")
	}
}

func TestLogLocationConflictsRemainUnknown(t *testing.T) {
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	history := logLocationHistory{}
	history.merge(logLocationHistory{"session": {CharacterName: "Pilot", Start: start.Unix(), End: start.Unix() + 100,
		Events: []logLocationEvent{{Second: start.Unix(), To: "Mamet"}, {Second: start.Unix(), To: "Mai"}}}})
	for _, withESI := range []bool{false, true} {
		esi := locationHistory{}
		if withESI {
			esi[1] = []locationPeriod{{CharacterID: 1, CharacterName: "Pilot", SystemID: 99, SystemName: "Cached", FirstSeen: start, LastSeen: start.Add(time.Hour)}}
		}
		if location, ok := newLocationResolver(esi, history).atName("Pilot", start.Add(time.Second)); ok {
			t.Fatal(fmt.Sprintf("conflicting logged locations chose %+v", location))
		}
	}
}

func TestLogLocationHistoryViewer(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	log, err := parseLog(strings.NewReader(movementLog))
	if err != nil {
		t.Fatal(err)
	}
	ui.logLocations = (&logSnapshot{Logs: []*parsedLog{log}}).locationSessions()
	content := logLocationHistoryContent(ui.logLocations).(*fyne.Container)
	var table *widget.Table
	for _, object := range content.Objects {
		if candidate, ok := object.(*widget.Table); ok {
			table = candidate
		}
	}
	if table == nil {
		t.Fatal("missing log coverage table")
	}
	if rows, columns := table.Length(); rows != 5 || columns != 4 {
		t.Fatalf("unexpected coverage table size %dx%d", rows, columns)
	}
	label := widget.NewLabel("")
	for column, want := range []string{"Pilot A", "Camal", "2026-10-19 02:00:30", "2026-10-19 02:10:00"} {
		table.UpdateCell(widget.TableCellID{Row: 1, Col: column}, label)
		if label.Text != want {
			t.Fatalf("column %d = %q, want %q", column, label.Text, want)
		}
	}
	ui.openLocationHistory()
	if ui.window.Canvas().Overlays().Top() == nil {
		t.Fatal("combined location history did not open")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_LOG_LOCATION_CAPTURE"))
}
