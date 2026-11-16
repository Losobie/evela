package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDockedNamesResolveCacheAndPersist(t *testing.T) {
	f, account := locationFixture(t)
	ctx := context.Background()
	f.locationStation.Store(60003760)
	for i := 0; i < 2; i++ {
		if err := f.service.refreshLocation(ctx, account); err != nil {
			t.Fatal(err)
		}
	}
	station := "Jita IV - Moon 4 - Caldari Navy Assembly Plant"
	if account.Location.StationName != station || f.stationRequests.Load() != 1 || !strings.Contains(locationLabel(account.Location), station) {
		t.Fatalf("station name was not resolved, displayed and cached: %+v", account.Location)
	}
	f.token = signedTestToken(t, f.key, func(claims map[string]any) { claims["scp"] = []string{shipScope, locationScope, structureScope} })
	account.Token.AccessToken = f.token
	account.Token.Scopes = []string{shipScope, locationScope, structureScope}
	f.locationStation.Store(0)
	f.locationStructure.Store(100000000001)
	for i := 0; i < 2; i++ {
		if err := f.service.refreshLocation(ctx, account); err != nil {
			t.Fatal(err)
		}
	}
	if account.Location.StructureName != "Pilot's Fortizar" || account.Location.StationName != "" || f.structureRequests.Load() != 1 {
		t.Fatalf("structure name was not resolved and cached independently of station: %+v", account.Location)
	}
	data, err := json.Marshal(f.service.config)
	if err != nil {
		t.Fatal(err)
	}
	var restored esiConfig
	if err := json.Unmarshal(data, &restored); err != nil || restored.Accounts[123].Location.StructureName != "Pilot's Fortizar" {
		t.Fatalf("resolved name did not survive settings serialization: %v", err)
	}
	// Expired private metadata must be rechecked, including one 401 renewal.
	key := structureNameKey{CharacterID: 123, StructureID: 100000000001}
	cached := f.service.structureNames[key]
	cached.until = f.service.now().Add(-time.Second)
	f.service.structureNames[key] = cached
	f.structureUnauthorized.Store(true)
	if err := f.service.refreshLocation(ctx, account); err != nil || f.structureRequests.Load() != 3 || f.tokenRequests.Load() != 1 || account.Location.DockedNameError != "" {
		t.Fatalf("expired structure name did not renew/retry: %v, %+v", err, account.Location)
	}
	// A different character cannot reuse the first character's private result.
	f.structureStatus.Store(http.StatusForbidden)
	other := &esiAccount{Ship: shipRecord{CharacterID: 456}, Token: account.Token}
	if name, err := f.service.lookupStructureName(ctx, other, 100000000001); name != "" || err == nil || f.structureRequests.Load() != 4 {
		t.Fatal("another character bypassed access checks using cached metadata")
	}
	f.locationStructure.Store(0)
	if err := f.service.refreshLocation(ctx, account); err != nil || account.Location.StructureName != "" || account.Location.StationName != "" {
		t.Fatal("undocking retained the previous docking name")
	}
}

func TestDockedNameFailuresRetainLocationAndRetry(t *testing.T) {
	for _, scenario := range []string{"station network", "missing structure scope", "structure forbidden", "structure network"} {
		t.Run(scenario, func(t *testing.T) {
			f, account := locationFixture(t)
			if scenario == "station network" {
				f.locationStation.Store(60003760)
				f.stationStatus.Store(http.StatusServiceUnavailable)
			} else {
				f.locationStructure.Store(100000000001)
				if scenario != "missing structure scope" {
					account.Token.Scopes = append(account.Token.Scopes, structureScope)
				}
				if scenario == "structure forbidden" {
					f.structureStatus.Store(http.StatusForbidden)
				} else if scenario == "structure network" {
					f.structureStatus.Store(http.StatusServiceUnavailable)
				}
			}
			if err := f.service.refreshLocation(context.Background(), account); err != nil {
				t.Fatal("optional name failure invalidated the location:", err)
			}
			location := account.Location
			if location.Error != "" || location.SystemName != "Jita" || location.DockedNameError == "" || len(f.service.config.LocationHistory[123]) != 1 {
				t.Fatalf("name failure discarded the confirmed system or observation: %+v", location)
			}
			label := locationLabel(location)
			want := "Jita · 100000000001"
			if scenario == "station network" {
				want = "Jita · 60003760"
			}
			if label != want {
				t.Fatalf("name failure should show only the numeric ID: %s", label)
			}
			if scenario == "missing structure scope" && (f.structureRequests.Load() != 0 || !strings.Contains(location.DockedNameError, "Sign in for structure names")) {
				t.Fatal("missing scope was not explained without a private request")
			}
			account.Token.Scopes = append(account.Token.Scopes, structureScope)
			f.stationStatus.Store(0)
			f.structureStatus.Store(0)
			if err := f.service.refreshLocation(context.Background(), account); err != nil || account.Location.DockedNameError != "" || account.Location.StationName == "" && account.Location.StructureName == "" {
				t.Fatalf("failed lookup was cached or did not recover: %v, %+v", err, account.Location)
			}
		})
	}
}
