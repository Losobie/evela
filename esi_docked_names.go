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

// Structure access belongs to the requesting character. Never let another
// character's cached result bypass that character's permissions.
type structureNameKey struct{ CharacterID, StructureID int64 }
type cachedStructureName struct {
	name  string
	until time.Time
}

func (service *esiService) lookupStationName(ctx context.Context, id int64) (string, error) {
	if name := service.stationNames[id]; name != "" {
		return name, nil
	}
	var station struct {
		Name string `json:"name"`
	}
	if err := service.requestJSON(ctx, http.MethodGet, fmt.Sprintf("%s/universe/stations/%d/", service.baseURL, id), "", nil, &station); err != nil {
		return "", fmt.Errorf("Look up station: %w", err)
	}
	if strings.TrimSpace(station.Name) == "" {
		return "", fmt.Errorf("ESI did not return a station name")
	}
	if service.stationNames == nil {
		service.stationNames = map[int64]string{}
	}
	service.stationNames[id] = station.Name
	return station.Name, nil
}

func (service *esiService) lookupStructureName(ctx context.Context, account *esiAccount, id int64) (string, error) {
	if !slices.Contains(account.Token.Scopes, structureScope) {
		return "", fmt.Errorf("Structure name unavailable: select %s when signing in", structureScope)
	}
	key := structureNameKey{CharacterID: account.Ship.CharacterID, StructureID: id}
	if cached := service.structureNames[key]; cached.name != "" && service.now().Before(cached.until) {
		return cached.name, nil
	}
	var structure struct {
		Name string `json:"name"`
	}
	endpoint := fmt.Sprintf("%s/universe/structures/%d/", service.baseURL, id)
	err := service.requestJSON(ctx, http.MethodGet, endpoint, account.Token.AccessToken, nil, &structure)
	var status *httpStatusError
	if errors.As(err, &status) && status.Status == http.StatusUnauthorized {
		if err = service.renew(ctx, account); err == nil {
			err = service.requestJSON(ctx, http.MethodGet, endpoint, account.Token.AccessToken, nil, &structure)
		}
	}
	if err != nil {
		if errors.As(err, &status) && status.Status == http.StatusForbidden {
			return "", fmt.Errorf("Structure name unavailable: this character needs docking access and structure read permission")
		}
		return "", fmt.Errorf("Look up structure: %w", err)
	}
	if strings.TrimSpace(structure.Name) == "" {
		return "", fmt.Errorf("ESI did not return a structure name")
	}
	if service.structureNames == nil {
		service.structureNames = map[structureNameKey]cachedStructureName{}
	}
	// Structures can be renamed or change access. Revalidate periodically.
	service.structureNames[key] = cachedStructureName{name: structure.Name, until: service.now().Add(5 * time.Minute)}
	return structure.Name, nil
}
