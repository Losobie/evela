package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func wormholeTestLog(t *testing.T, messages map[int]string) (*parsedLog, time.Time) {
	t.Helper()
	start := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	var input strings.Builder
	input.WriteString("Listener: Pilot A\nSession Started: " + start.Format(logTimeLayout) + "\n")
	// Reversed source order verifies the heuristic uses timestamps, including
	// when a notification appears before its preceding jump in an export.
	for second := 200; second >= 0; second-- {
		if message, ok := messages[second]; ok {
			fmt.Fprintf(&input, "[ %s ] %s\n", start.Add(time.Duration(second)*time.Second).Format(logTimeLayout), message)
		}
	}
	log, err := parseLog(strings.NewReader(input.String()))
	if err != nil {
		t.Fatal(err)
	}
	return log, start
}

func TestWormholeLocationRecentJumpThreshold(t *testing.T) {
	for _, jump := range []string{
		"(notify) Jumping from Mamet to Hoshoun",
		"(notify) A Conduit Field activated by Pilot jumps you to Hoshoun.",
		"(notify) Activating a Conduit Field jumps you to Hoshoun, bringing along 1 passenger.",
	} {
		for _, second := range []int{0, 29, 30, 31} {
			t.Run(fmt.Sprintf("%s/+%d", jump, second), func(t *testing.T) {
				// Keep separate entries at second zero for the simultaneous case.
				log, start := wormholeTestLog(t, map[int]string{0: jump, 100: "(combat) 100 to Enemy - Ogre I - Hits"})
				hint, ok := parseLogLocation(spatialPhenomenaMessage, start.Unix()+int64(second))
				if !ok || !hint.Inferred || hint.To != "Wormhole" {
					t.Fatalf("notification did not produce a weak location hint: %+v", hint)
				}
				log.LocationEvents = append(log.LocationEvents, hint)
				history := (&logSnapshot{Logs: []*parsedLog{log}}).locationSessions()
				resolver := newLocationResolver(nil, history)
				location, known := resolver.atName("Pilot A", start.Add(time.Duration(second+1)*time.Second))
				want := "Hoshoun"
				if second > 30 {
					want = "Wormhole"
				}
				if !known || location.SystemName != want {
					t.Fatalf("+%d seconds: %+v, known=%v, want %s", second, location, known, want)
				}
				for _, period := range history.periods() {
					if period.SystemName == "Wormhole" && second <= 30 {
						t.Fatal("history viewer retained a recent-jump-suppressed hint")
					}
				}
			})
		}
	}
}

func TestWormholeLocationNoJumpESIPriorityAndSessionBounds(t *testing.T) {
	log, start := wormholeTestLog(t, map[int]string{
		31:  "(notify) " + spatialPhenomenaMessage,
		32:  "(combat) 100 to Enemy - Ogre I - Hits",
		100: "(chat) End of session coverage",
	})
	history := (&logSnapshot{Logs: []*parsedLog{log}}).locationSessions()
	resolver := newLocationResolver(nil, history)
	for _, second := range []int{31, 32, 100, 145} {
		location, ok := resolver.atName("Pilot A", start.Add(time.Duration(second)*time.Second))
		if !ok || location.SystemName != "Wormhole" || location.SystemID != 0 || location.Source != inferredLogLocationSource {
			t.Fatalf("untracked wormhole at +%d: %+v, known=%v", second, location, ok)
		}
	}
	for _, second := range []int{0, 30, 146} {
		if location, ok := resolver.atName("Pilot A", start.Add(time.Duration(second)*time.Second)); ok {
			t.Fatalf("hint extrapolated outside its session / timestamp at +%d: %+v", second, location)
		}
	}
	if _, ok := resolver.atName("Other pilot", start.Add(32*time.Second)); ok {
		t.Fatal("wormhole inference leaked to another character")
	}
	esi := locationHistory{1: {{CharacterID: 1, CharacterName: "Pilot A", SystemID: 31000001, SystemName: "J123456", FirstSeen: start.Add(31 * time.Second), LastSeen: start.Add(32 * time.Second)}}}
	resolver = newLocationResolver(esi, history)
	if location, ok := resolver.atName("Pilot A", start.Add(32*time.Second)); !ok || location.SystemName != "J123456" || location.SystemID != 31000001 {
		t.Fatalf("inferred Wormhole replaced ESI: %+v", location)
	}
	if location, ok := resolver.atName("Pilot A", start.Add(78*time.Second)); !ok || location.SystemName != "Wormhole" {
		t.Fatalf("expired ESI data blocked the generic fallback: %+v", location)
	}
	// A matching notification mentioned in chat is never location evidence.
	chat, _ := wormholeTestLog(t, map[int]string{31: "(chat) " + spatialPhenomenaMessage})
	if len(chat.LocationEvents) != 0 {
		t.Fatal("chat text produced a Wormhole hint")
	}
}

func TestWormholeLocationSupersedesOldJumpAndYieldsToNewJump(t *testing.T) {
	log, start := wormholeTestLog(t, map[int]string{
		0:   "(notify) Jumping from Mamet to Hoshoun",
		20:  "(notify) " + spatialPhenomenaMessage,
		31:  "(notify) " + spatialPhenomenaMessage,
		40:  "(notify) " + spatialPhenomenaMessage,
		60:  "(notify) Jumping from Hoshoun to Jita",
		90:  "(notify) " + spatialPhenomenaMessage,
		100: "(combat) 100 to Enemy - Ogre I - Hits",
	})
	resolver := newLocationResolver(nil, (&logSnapshot{Logs: []*parsedLog{log}}).locationSessions())
	for second, want := range map[int]string{20: "Hoshoun", 30: "Hoshoun", 31: "Wormhole", 40: "Wormhole", 59: "Wormhole", 60: "Jita", 90: "Jita", 100: "Jita"} {
		if location, ok := resolver.atName("Pilot A", start.Add(time.Duration(second)*time.Second)); !ok || location.SystemName != want {
			t.Fatalf("at +%d: %+v, known=%v, want %s", second, location, ok, want)
		}
	}
	// A jump in another overlapping export still suppresses a hint within 30s.
	other := logLocationHistory{"other session": {CharacterName: "Pilot A", Start: start.Unix(), End: start.Unix() + 100,
		Events: []logLocationEvent{{Second: start.Unix() + 30, To: "Amarr"}}}}
	other.merge((&logSnapshot{Logs: []*parsedLog{log}}).locationSessions())
	if location, ok := newLocationResolver(nil, other).atName("Pilot A", start.Add(40*time.Second)); !ok || location.SystemName != "Amarr" {
		t.Fatalf("cross-export recent jump was ignored: %+v", location)
	}
}

func TestWormholeLocationsReachFiltersEngagementsAndEventDetails(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	log, start := wormholeTestLog(t, map[int]string{
		0:  "(notify) Jumping from Mamet to Hoshoun",
		31: "(notify) " + spatialPhenomenaMessage,
		32: "(combat) 200 to Enemy[C](Raven) - Ogre I - Hits",
	})
	ui.snapshot = &logSnapshot{Logs: []*parsedLog{log}, Characters: []string{"Pilot A"}}
	ui.locationHistory, ui.logLocations, ui.shipHistory = nil, nil, nil
	ui.applyGraphRange(start, start.Add(60*time.Second))
	ui.setAllDamageFilterOptions(filterSystem, false)
	ui.setDamageFilterOption(filterSystem, "system-name:wormhole", true)
	if filteredTotal(ui) != 200 {
		t.Fatal("inferred Wormhole system did not filter damage")
	}
	rows, err := buildEngagements(context.Background(), ui.snapshot, ui.resolvedLocations(), nil)
	if err != nil || len(rows) != 1 || rows[0].Summary != "Wormhole - Shooting Raven" {
		t.Fatalf("engagement did not use inferred Wormhole: %+v, %v", rows, err)
	}
	event := eventRow{logEvent: log.Events[1], Character: "Pilot A"}
	if text := ui.events.eventDetails(event); !strings.Contains(text, "Current system: Wormhole") || !strings.Contains(text, "Location hint: Wormhole") || strings.Contains(text, "Activity: Travel") {
		t.Fatalf("notification details hid the assumption: %s", text)
	}
	ui.locationHistory = locationHistory{1: {{CharacterID: 1, CharacterName: "Pilot A", SystemID: 31000001, SystemName: "J123456", FirstSeen: start.Add(31 * time.Second), LastSeen: start.Add(32 * time.Second)}}}
	ui.rebuildView()
	if filteredTotal(ui) != 0 || !strings.Contains(ui.events.eventDetails(event), "Current system: J123456") {
		t.Fatal("new ESI data did not replace the inferred location")
	}
}

func TestWormholeLocationCacheAndHistoryRoundTrip(t *testing.T) {
	folder := t.TempDir()
	writeCacheLog(t, folder, "20261104_wormhole.txt", "Listener: Pilot A\n[ 2026.11.04 12:00:31 ] (notify) "+spatialPhenomenaMessage+"\n[ 2026.11.04 12:00:32 ] (combat) 100 to Enemy - Ogre I - Hits\n")
	path := filepath.Join(t.TempDir(), "logs.sqlite")
	cache := testLogCache(t, path, logParserVersion)
	first := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	cache.close()
	cache = testLogCache(t, path, logParserVersion)
	second := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if first.ParsedFiles != 1 || second.CachedFiles != 1 || len(second.Logs[0].LocationEvents) != 1 || !second.Logs[0].LocationEvents[0].Inferred {
		t.Fatal("SQLite restart lost the inferred location marker")
	}
	history := second.locationSessions()
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	var restored logLocationHistory
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.merge(history) {
		t.Fatal("unchanged hints were duplicated on history refresh")
	}
	at := time.Date(2026, 11, 4, 12, 0, 32, 0, time.UTC)
	if location, ok := newLocationResolver(nil, restored).atName("Pilot A", at); !ok || location.SystemName != "Wormhole" {
		t.Fatalf("saved location history lost its hint: %+v", location)
	}
}
