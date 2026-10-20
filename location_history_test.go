package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLocationHistoryPeriodsAndLookup(t *testing.T) {
	service := newESIService("")
	start := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)
	location := locationRecord{CharacterID: 123, CharacterName: "Pilot", SystemID: 30000142, CheckedAt: start}
	service.rememberLocation(location, false)
	location.CheckedAt = start.Add(30 * time.Second)
	location.SystemName = "Jita"
	service.rememberLocation(location, true)
	if p := service.config.LocationHistory[123]; len(p) != 1 || p[0].Observations != 2 || p[0].SystemName != "Jita" {
		t.Fatalf("consecutive observations not combined: %+v", p)
	}
	// Duplicate/backwards timestamps and failed queries cannot extend coverage.
	service.rememberLocation(location, true)
	location.CheckedAt = start.Add(20 * time.Second)
	service.rememberLocation(location, true)
	location.CheckedAt = start.Add(45 * time.Second)
	location.Error = "failed"
	service.rememberLocation(location, true)
	location.Error = ""
	location.CheckedAt = start.Add(60 * time.Second)
	location.SystemID, location.SystemName = 30002187, "Amarr"
	service.rememberLocation(location, true)
	location.CheckedAt = start.Add(90 * time.Second)
	location.StationID = 60008494
	service.rememberLocation(location, true)
	location.CheckedAt = start.Add(120 * time.Second)
	location.StationID, location.StructureID = 0, 100000000001
	service.rememberLocation(location, true)
	location.CheckedAt = start.Add(150 * time.Second)
	location.StructureID = 0
	service.rememberLocation(location, true)
	location.CheckedAt = start.Add(4*time.Minute + 1*time.Second)
	service.rememberLocation(location, true) // gap over 90 seconds
	location.CheckedAt = start.Add(4*time.Minute + 31*time.Second)
	service.rememberLocation(location, false) // failure/reconnect/restart
	history := service.config.LocationHistory
	if len(history[123]) != 7 || history[123][0].Observations != 2 {
		t.Fatalf("wrong location periods: %+v", history[123])
	}
	for _, tc := range []struct {
		seconds int
		system  int64
		known   bool
	}{
		{-1, 0, false}, {0, 30000142, true}, {59, 30000142, true}, {60, 30002187, true},
		{195, 30002187, true}, {196, 0, false},
	} {
		p, ok := history.atName(" pILOT ", start.Add(time.Duration(tc.seconds)*time.Second))
		if ok != tc.known || ok && p.SystemID != tc.system {
			t.Fatalf("lookup at %d = %+v, %v", tc.seconds, p, ok)
		}
	}
	copy := history.clone()
	copy[123][0].SystemName = "changed"
	if history[123][0].SystemName != "Jita" {
		t.Fatal("clone shares history storage")
	}
	history[456] = []locationPeriod{{CharacterID: 456, CharacterName: "Pilot", FirstSeen: start, LastSeen: start}}
	if _, ok := history.atName("Pilot", start); ok {
		t.Fatal("ambiguous name attributed to a location")
	}
}

func locationFixture(t *testing.T) (*esiFixture, *esiAccount) {
	t.Helper()
	f := newESIFixture(t)
	f.token = signedTestToken(t, f.key, func(c map[string]any) { c["scp"] = []string{shipScope, locationScope} })
	f.locationSystem.Store(30000142)
	account := &esiAccount{Ship: shipRecord{CharacterID: 123, CharacterName: "Test Pilot"},
		Token: esiToken{AccessToken: f.token, RefreshToken: "original-refresh", ExpiresAt: time.Now().Add(time.Hour), Scopes: []string{shipScope, locationScope}}}
	f.service.config.Accounts[123] = account
	useShipHistoryTestClock(f.service)
	return f, account
}

func TestLocationRefreshPersistenceAndIndependentFailures(t *testing.T) {
	f, account := locationFixture(t)
	refresh := func() {
		t.Helper()
		if err := f.service.refreshShips(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	refresh()
	refresh()
	if p := f.service.config.LocationHistory[123]; len(p) != 1 || p[0].Observations != 2 || p[0].SystemName != "Jita" || f.systemRequests.Load() != 1 {
		t.Fatalf("missing location observations or cache: %+v", p)
	}
	f.locationSystem.Store(30002187)
	refresh()
	f.locationStatus.Store(http.StatusServiceUnavailable)
	if err := f.service.refreshShips(context.Background()); err == nil || account.Ship.Error != "" || account.Location.Error == "" {
		t.Fatal("location failure invalidated ship or went unreported")
	}
	f.locationStatus.Store(0)
	refresh()
	if len(f.service.config.LocationHistory[123]) != 3 {
		t.Fatal("location failure did not split observations")
	}
	f.shipStatus.Store(http.StatusServiceUnavailable)
	if err := f.service.refreshShips(context.Background()); err == nil || account.Location.Error != "" {
		t.Fatal("ship failure invalidated location")
	}
	if f.service.config.LocationHistory[123][2].Observations != 2 {
		t.Fatal("ship failure discarded successful location")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if p, ok := loaded.config.LocationHistory.at(123, f.service.config.LocationHistory[123][0].FirstSeen); !ok || p.SystemName != "Jita" || p.Observations != 2 {
		t.Fatal("encrypted location history failed to round-trip")
	}
	// A restart splits observations, while historical IDs/names remain intact.
	f.service.config = loaded.config
	f.shipStatus.Store(0)
	refresh()
	if len(f.service.config.LocationHistory[123]) != 4 {
		t.Fatal("restart extended an old period")
	}
	delete(f.service.config.Accounts, 123)
	if err := f.service.save(); err != nil {
		t.Fatal(err)
	}
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(loaded.config.Accounts) != 0 || len(loaded.config.LocationHistory[123]) != 4 {
		t.Fatal("disconnect removed location history")
	}
	if _, err := f.service.importTokens(context.Background(), f.token, ""); err != nil {
		t.Fatal(err)
	}
	refresh()
	if len(f.service.config.LocationHistory[123]) != 5 {
		t.Fatal("token replacement did not retain history and split period")
	}
}

func TestLocationLookupFailureRetainsObservationAndRetriesDisconnectedMetadata(t *testing.T) {
	f, account := locationFixture(t)
	f.systemStatus.Store(http.StatusServiceUnavailable)
	if err := f.service.refreshShips(context.Background()); err == nil {
		t.Fatal("name lookup failure was not reported")
	}
	first := f.service.config.LocationHistory[123][0]
	if first.SystemID != 30000142 || first.SystemName != "" || account.Location.Error != "" {
		t.Fatal("name failure discarded confirmed location")
	}
	loaded := newESIService(f.service.file)
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(loaded.config.LocationHistory[123]) != 1 {
		t.Fatal("unnamed location was not saved")
	}
	delete(f.service.config.Accounts, 123)
	f.systemStatus.Store(0)
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := f.service.config.LocationHistory[123][0]
	if got.SystemName != "Jita" || !got.FirstSeen.Equal(first.FirstSeen) || !got.LastSeen.Equal(first.LastSeen) {
		t.Fatal("metadata retry changed observation coverage")
	}
}

func TestLocationLegacyScopesUnauthorizedAndRateLimit(t *testing.T) {
	f, account := locationFixture(t)
	account.Token.Scopes = nil // Saved by a previous version, already granted location.
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.locationRequests.Load() != 1 || len(account.Token.Scopes) != 2 {
		t.Fatal("legacy token's granted scopes were not discovered")
	}
	f.locationUnauthorized.Store(true)
	if err := f.service.refreshShips(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.locationRequests.Load() != 3 || f.tokenRequests.Load() != 1 {
		t.Fatal("location 401 did not renew and retry once")
	}
	f.locationStatus.Store(http.StatusTooManyRequests)
	if err := f.service.refreshShips(context.Background()); err == nil {
		t.Fatal("location rate limit not reported")
	}
	before := f.locationRequests.Load()
	if err := f.service.refreshShips(context.Background()); err == nil || f.locationRequests.Load() != before {
		t.Fatal("location ignored ESI backoff")
	}
	account.Token.Scopes = []string{shipScope}
	if err := f.service.refreshLocation(context.Background(), account); err != nil || !strings.Contains(account.Location.Error, "sign in again") || f.locationRequests.Load() != before {
		t.Fatal("ship-only token lost compatibility or lacks reauthorization guidance")
	}
}
