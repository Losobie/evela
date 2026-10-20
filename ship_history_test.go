package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShipHistoryPeriodsAndTimestampCorrelation(t *testing.T) {
	service := newESIService("")
	start := time.Date(2026, 10, 17, 2, 0, 0, 0, time.FixedZone("test", -7*3600))
	ship := shipRecord{CharacterID: 123, CharacterName: "Test Pilot", ShipID: 1001, TypeID: 29990, TypeName: "Loki", ShipName: "First ship", CheckedAt: start}
	service.rememberShip(ship, false)
	ship.CheckedAt = start.Add(30 * time.Second)
	service.rememberShip(ship, true)
	history := service.config.ShipHistory
	if len(history[123]) != 1 || history[123][0].Observations != 2 || history[123][0].FirstSeen.Location() != time.UTC {
		t.Fatalf("consecutive observations did not combine in UTC: %+v", history)
	}
	for _, offset := range []time.Duration{0, 15 * time.Second, 30 * time.Second} {
		period, ok := history.atName("  TEST PILOT ", start.Add(offset))
		if !ok || period.ShipID != 1001 || period.TypeID != 29990 {
			t.Fatalf("damage timestamp %s did not match the observed ship: %+v", offset, period)
		}
	}

	// A different ship of the same hull, a rename, a hull change, a failed poll,
	// and a long pause must each start a new period.
	ship.CheckedAt, ship.ShipID = start.Add(45*time.Second), 1002
	service.rememberShip(ship, true)
	ship.CheckedAt, ship.ShipName = start.Add(60*time.Second), "Renamed ship"
	service.rememberShip(ship, true)
	ship.CheckedAt, ship.TypeID, ship.TypeName = start.Add(75*time.Second), 29984, "Tengu"
	service.rememberShip(ship, true)
	ship.CheckedAt = start.Add(90 * time.Second)
	service.rememberShip(ship, false)
	ship.CheckedAt = start.Add(5 * time.Minute)
	service.rememberShip(ship, true)
	if len(history[123]) != 6 {
		t.Fatalf("ship changes or gaps were merged: %+v", history[123])
	}
	for _, offset := range []time.Duration{-time.Second, 136 * time.Second, 6 * time.Minute} {
		if _, ok := history.at(123, start.Add(offset)); ok {
			t.Fatalf("unobserved timestamp %s was assigned a ship", offset)
		}
	}
	if _, ok := history.at(999, start); ok {
		t.Fatal("unknown character matched history")
	}
	if _, ok := history.atName("Other Pilot", start); ok {
		t.Fatal("unknown name matched history")
	}
	for _, record := range []shipRecord{
		ship, // Duplicate timestamp.
		{CharacterID: 123, CharacterName: "Test Pilot", TypeID: 29990, CheckedAt: start.Add(-time.Hour)},
		{CharacterID: 123, CharacterName: "Test Pilot", TypeID: 29990, CheckedAt: start.Add(time.Hour), Error: "ESI unavailable"},
		{CharacterID: 0, CharacterName: "Test Pilot", TypeID: 29990, CheckedAt: start.Add(time.Hour)},
		{CharacterID: 123, CharacterName: "", TypeID: 29990, CheckedAt: start.Add(time.Hour)},
		{CharacterID: 123, CharacterName: "Test Pilot", TypeID: 0, CheckedAt: start.Add(time.Hour)},
		{CharacterID: 123, CharacterName: "Test Pilot", TypeID: 29990},
	} {
		service.rememberShip(record, true)
	}
	if len(history[123]) != 6 || history[123][5].Observations != 1 {
		t.Fatal("invalid, duplicate or backwards observations changed history")
	}
	snapshot := history.clone()
	ship.CheckedAt = start.Add(5*time.Minute + 30*time.Second)
	service.rememberShip(ship, true)
	if snapshot[123][5].Observations != 1 || history[123][5].Observations != 2 {
		t.Fatal("worker mutated the UI's history snapshot")
	}
	ship.CharacterID = 456
	service.rememberShip(ship, false)
	if _, ok := history.atName("Test Pilot", ship.CheckedAt); ok {
		t.Fatal("ambiguous character names should stay unknown")
	}
}

func TestShipHistoryAssumesLastObservedShipFor45Seconds(t *testing.T) {
	start := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	history := shipHistory{123: {
		{CharacterID: 123, CharacterName: "Pilot", ShipID: 1, FirstSeen: start, LastSeen: start.Add(30 * time.Second)},
		{CharacterID: 123, CharacterName: "Pilot", ShipID: 2, FirstSeen: start.Add(time.Minute), LastSeen: start.Add(time.Minute)},
		{CharacterID: 123, CharacterName: "Pilot", ShipID: 3, FirstSeen: start.Add(3 * time.Minute), LastSeen: start.Add(200 * time.Second)},
	}}
	for _, item := range []struct {
		offset time.Duration
		shipID int64
	}{
		{-time.Nanosecond, 0}, // No assumption before the first observation.
		{0, 1},
		{30 * time.Second, 1},
		{59 * time.Second, 1},
		{time.Minute, 2}, // The new ship immediately supersedes the old ship.
		{75 * time.Second, 2},
		{105 * time.Second, 2}, // Inclusive 45-second limit after a single sighting.
		{105*time.Second + time.Nanosecond, 0},
		{150 * time.Second, 0}, // Long gaps remain unknown.
		{3 * time.Minute, 3},
		{245 * time.Second, 3},
		{245*time.Second + time.Nanosecond, 0},
	} {
		period, ok := history.atName(" PILOT ", start.Add(item.offset))
		if ok != (item.shipID != 0) || period.ShipID != item.shipID {
			t.Fatalf("at %s: got ship %d / %v, want %d", item.offset, period.ShipID, ok, item.shipID)
		}
	}
	if !history[123][2].LastSeen.Equal(start.Add(200 * time.Second)) {
		t.Fatal("correlation changed the recorded observation timestamp")
	}
}

func TestShipHistoryRefreshPersistenceAndReconnect(t *testing.T) {
	f := newESIFixture(t)
	useShipHistoryTestClock(f.service)
	account := &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"}, Token: esiToken{AccessToken: f.token, ExpiresAt: time.Now().Add(time.Hour)}}
	f.service.config.Accounts[123] = account
	refresh := func() {
		t.Helper()
		if err := f.service.refreshShips(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	refresh()
	if got := f.service.config.ShipHistory[123]; len(got) != 1 || got[0].Observations != 2 || got[0].ShipID != 1000000001 {
		t.Fatalf("ESI sightings were not recorded: %+v", got)
	}
	f.shipItem.Store(1000000002)
	refresh()
	f.shipType.Store(29984)
	refresh()
	f.shipStatus.Store(500)
	if err := f.service.refreshShips(context.Background()); err == nil {
		t.Fatal("failed poll was not reported")
	}
	if len(f.service.config.ShipHistory[123]) != 3 {
		t.Fatal("failed ship request added a sighting")
	}
	f.shipStatus.Store(0)
	refresh()
	if len(f.service.config.ShipHistory[123]) != 4 {
		t.Fatal("poll after failure extended an older period")
	}
	first := f.service.config.ShipHistory[123][0]
	if first.GroupID != 963 || first.ClassName != "Strategic Cruiser" {
		t.Fatal("observations did not save ship class metadata")
	}
	data, err := os.ReadFile(f.service.file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Test Pilot") || strings.Contains(string(data), "Loki") {
		t.Fatal("ship history was stored in plaintext")
	}
	restarted := newESIService(f.service.file)
	if err := restarted.load(); err != nil {
		t.Fatal(err)
	}
	if period, ok := restarted.config.ShipHistory.at(123, first.FirstSeen); !ok || period.ShipID != first.ShipID || period.Observations != 2 {
		t.Fatal("history did not survive restart")
	}
	if restarted.config.ShipHistory[123][0].ClassName != "Strategic Cruiser" {
		t.Fatal("historical ship class did not survive restart")
	}
	restarted.baseURL, restarted.discoveryURL = f.service.baseURL, f.service.discoveryURL
	restarted.now = f.service.now
	f.service = restarted
	refresh()
	if len(f.service.config.ShipHistory[123]) != 5 {
		t.Fatal("restart bridged a period across unobserved time")
	}
	delete(f.service.config.Accounts, 123) // Same operation as Disconnect.
	if err := f.service.save(); err != nil {
		t.Fatal(err)
	}
	if err := f.service.load(); err != nil {
		t.Fatal(err)
	}
	if len(f.service.config.Accounts) != 0 || len(f.service.config.ShipHistory[123]) != 5 {
		t.Fatal("disconnect removed history")
	}
	if _, err := f.service.importTokens(context.Background(), f.token, ""); err != nil {
		t.Fatal(err)
	}
	refresh()
	if len(f.service.config.ShipHistory[123]) != 6 {
		t.Fatal("reconnecting erased history or bridged a disconnected interval")
	}
}

func TestShipHistoryKeepsObservationWhenTypeLookupFails(t *testing.T) {
	f := newESIFixture(t)
	f.typeStatus.Store(503)
	f.service.config.Accounts[123] = &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"}, Token: esiToken{AccessToken: f.token, ExpiresAt: time.Now().Add(time.Hour)}}
	if err := f.service.refreshShips(context.Background()); err == nil {
		t.Fatal("hull-name lookup failure was not reported")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if got := loaded.config.ShipHistory[123]; len(got) != 1 || got[0].TypeID != 29990 || got[0].TypeName != "" || got[0].ShipID != 1000000001 {
		t.Fatalf("valid ship query was lost when type-name lookup failed: %+v", got)
	}
}

func TestShipHistoryMigratesLastKnownShip(t *testing.T) {
	service := newESIService(filepath.Join(t.TempDir(), "esi.dat"))
	seen := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	service.config.Accounts[123] = &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot", TypeID: 29990, TypeName: "Loki", CheckedAt: seen, Error: "Last refresh failed"}}
	service.config.Accounts[456] = &esiAccount{Ship: shipRecord{CharacterID: 456, CharacterName: "Never observed"}}
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if err := service.load(); err != nil {
		t.Fatal(err)
	}
	history := service.config.ShipHistory
	if got := history[123]; len(got) != 1 || !got[0].FirstSeen.Equal(seen) || !got[0].LastSeen.Equal(seen) || got[0].Observations != 1 || got[0].ShipID != 0 {
		t.Fatalf("legacy observation was not preserved without inventing coverage: %+v", got)
	}
	if len(history[456]) != 0 {
		t.Fatal("unobserved legacy character was assigned history")
	}
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if err := service.load(); err != nil {
		t.Fatal(err)
	}
	if len(service.config.ShipHistory[123]) != 1 {
		t.Fatal("loading migrated settings duplicated history")
	}
}

func TestShipHistorySaveFailureIsReportedAndRetried(t *testing.T) {
	f := newESIFixture(t)
	useShipHistoryTestClock(f.service)
	f.service.config.Accounts[123] = &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"}, Token: esiToken{AccessToken: f.token, ExpiresAt: time.Now().Add(time.Hour)}}
	file := f.service.file
	f.service.file = t.TempDir() // Replacing a directory with a settings file fails.
	if err := f.service.refreshShips(context.Background()); err == nil || !strings.Contains(err.Error(), "Save ESI connections") {
		t.Fatalf("history persistence failure was not reported: %v", err)
	}
	first := f.service.config.ShipHistory[123][0].FirstSeen
	f.service.file = file
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded := newESIService(file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if got := loaded.config.ShipHistory[123]; len(got) != 1 || got[0].Observations != 2 || !got[0].FirstSeen.Equal(first) {
		t.Fatalf("failed save lost the pending observation: %+v", got)
	}
}

func TestShipHistorySavesCompletedObservationsOnCancellation(t *testing.T) {
	f := newESIFixture(t)
	for _, id := range []int64{123, 456} {
		f.service.config.Accounts[id] = &esiAccount{Ship: shipRecord{CharacterID: id, CharacterName: "Test Pilot"}, Token: esiToken{AccessToken: f.token, ExpiresAt: time.Now().Add(time.Hour)}}
	}
	f.service.shipTypes[29990] = shipTypeInfo{Name: "Loki", GroupID: 963}
	f.service.groupNames[963] = "Strategic Cruiser"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.service.now = func() time.Time {
		cancel() // The first ship response completed; the second poll is cancelled.
		return time.Now()
	}
	if err := f.service.refreshShips(ctx); err == nil {
		t.Fatal("cancellation was not reported")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(loaded.config.ShipHistory.periods()) != 1 || f.shipRequests.Load() != 1 {
		t.Fatal("cancellation lost the completed observation or recorded an incomplete poll")
	}
}

// Windows can return the same wall-clock value for back-to-back mock requests.
// Advance explicitly so these tests exercise distinct observation timestamps.
func useShipHistoryTestClock(service *esiService) {
	at := time.Now().UTC()
	service.now = func() time.Time {
		at = at.Add(time.Second)
		return at
	}
}
