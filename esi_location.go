package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

type locationRecord struct {
	CharacterID   int64
	CharacterName string
	SystemID      int64
	SystemName    string
	StationID     int64
	StructureID   int64
	CheckedAt     time.Time
	Error         string
	NameError     string `json:"-"`
}

func (service *esiService) refreshLocation(ctx context.Context, account *esiAccount) error {
	// Legacy saved tokens predate stored scopes. Validate them once to discover
	// existing grants without requiring another sign-in if location was granted.
	if account.Token.Scopes == nil && ctx.Err() == nil {
		identity, err := service.validateToken(ctx, account.Token.AccessToken)
		if err == nil && identity.ID == account.Ship.CharacterID {
			account.Token.Scopes = identity.Scopes
		}
	}
	if !slices.Contains(account.Token.Scopes, locationScope) {
		account.Location.Error = "Location unavailable: enable " + locationScope + " in your EVE application and sign in again."
		return nil // Existing ship-only connections remain usable.
	}
	now := time.Now()
	if account.Token.ExpiresAt.Before(now.Add(time.Minute)) && (account.Token.RefreshToken != "" || !account.Token.ExpiresAt.After(now)) {
		if err := service.renew(ctx, account); err != nil {
			return fmt.Errorf("Location: %w", err)
		}
	}
	var response struct {
		SystemID    int64 `json:"solar_system_id"`
		StationID   int64 `json:"station_id"`
		StructureID int64 `json:"structure_id"`
	}
	endpoint := fmt.Sprintf("%s/characters/%d/location/", service.baseURL, account.Ship.CharacterID)
	err := service.requestJSON(ctx, http.MethodGet, endpoint, account.Token.AccessToken, nil, &response)
	var status *httpStatusError
	if errors.As(err, &status) && status.Status == http.StatusUnauthorized {
		if err = service.renew(ctx, account); err == nil {
			err = service.requestJSON(ctx, http.MethodGet, endpoint, account.Token.AccessToken, nil, &response)
		}
	}
	if err != nil {
		if errors.As(err, &status) && status.Status == http.StatusForbidden {
			return fmt.Errorf("location access denied; enable %s and sign in again", locationScope)
		}
		return fmt.Errorf("Location: %w", err)
	}
	if response.SystemID <= 0 {
		return fmt.Errorf("ESI did not return a solar system")
	}
	location := locationRecord{CharacterID: account.Ship.CharacterID, CharacterName: account.Ship.CharacterName,
		SystemID: response.SystemID, StationID: response.StationID, StructureID: response.StructureID, CheckedAt: service.now().UTC()}
	continuous := account.Location.Error == "" && !account.Location.CheckedAt.IsZero()
	// A public name lookup failure never invalidates a confirmed system ID.
	location.SystemName, err = service.lookupSystemName(ctx, location.SystemID)
	if err != nil {
		location.NameError = err.Error()
	}
	account.Location = location
	service.rememberLocation(location, continuous)
	return nil
}

func (service *esiService) lookupSystemName(ctx context.Context, id int64) (string, error) {
	if name := service.systemNames[id]; name != "" {
		return name, nil
	}
	var system struct {
		Name string `json:"name"`
	}
	if err := service.requestJSON(ctx, http.MethodGet, fmt.Sprintf("%s/universe/systems/%d/?language=en", service.baseURL, id), "", nil, &system); err != nil {
		return "", fmt.Errorf("Look up system: %w", err)
	}
	if strings.TrimSpace(system.Name) == "" {
		return "", fmt.Errorf("ESI did not return a solar system name")
	}
	if service.systemNames == nil {
		service.systemNames = map[int64]string{}
	}
	service.systemNames[id] = system.Name
	return system.Name, nil
}

func (service *esiService) refreshLocationHistoryMetadata(ctx context.Context) error {
	attempted := map[int64]bool{}
	for _, account := range service.config.Accounts {
		if account.Location.NameError != "" {
			attempted[account.Location.SystemID] = true
		}
	}
	var firstError error
	for _, periods := range service.config.LocationHistory {
		for index := range periods {
			period := &periods[index]
			if period.SystemID <= 0 || period.SystemName != "" {
				continue
			}
			if !attempted[period.SystemID] {
				attempted[period.SystemID] = true
				if _, err := service.lookupSystemName(ctx, period.SystemID); err != nil && firstError == nil {
					firstError = fmt.Errorf("Location history metadata: %w", err)
				}
			}
			if name := service.systemNames[period.SystemID]; name != "" {
				period.SystemName = name
			}
		}
	}
	return firstError
}

func locationLabel(location locationRecord) string {
	name := location.SystemName
	if name == "" && location.SystemID > 0 {
		name = fmt.Sprintf("System %d", location.SystemID)
	}
	if name == "" {
		name = "Location unavailable"
	} else if location.Error != "" || time.Since(location.CheckedAt) >= locationHistoryMaxGap {
		name += " (stale)"
	}
	if location.StationID > 0 {
		name += fmt.Sprintf(" · Station %d", location.StationID)
	} else if location.StructureID > 0 {
		name += fmt.Sprintf(" · Structure %d", location.StructureID)
	}
	if location.Error != "" {
		name += " · " + location.Error
	}
	if location.NameError != "" {
		name += " · " + location.NameError
	}
	return name
}
