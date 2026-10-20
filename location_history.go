package main

import (
	"sort"
	"time"
)

const locationHistoryMaxGap = shipHistoryMaxGap
const locationHistoryAssumptionWindow = shipHistoryAssumptionWindow

// Periods retain actual observations, independently of saved sign-ins. Docking,
// undocking and changing structures split periods even within the same system.
type locationPeriod struct {
	CharacterID   int64
	CharacterName string
	SystemID      int64
	SystemName    string
	StationID     int64
	StructureID   int64
	FirstSeen     time.Time
	LastSeen      time.Time
	Observations  int
	Source        string `json:",omitempty"`
}

type locationHistory map[int64][]locationPeriod

func (history locationHistory) indexByName() map[string]locationHistory {
	byName := map[string]locationHistory{}
	for id, periods := range history {
		for _, period := range periods {
			key := characterKey(period.CharacterName)
			if byName[key] == nil {
				byName[key] = locationHistory{}
			}
			byName[key][id] = append(byName[key][id], period)
		}
	}
	return byName
}

func (service *esiService) rememberLocation(location locationRecord, continuous bool) {
	if location.CharacterID <= 0 || characterKey(location.CharacterName) == "" || location.SystemID <= 0 || location.CheckedAt.IsZero() || location.Error != "" {
		return
	}
	if service.config.LocationHistory == nil {
		service.config.LocationHistory = locationHistory{}
	}
	history := service.config.LocationHistory
	periods := history[location.CharacterID]
	at := location.CheckedAt.UTC()
	if len(periods) > 0 {
		last := &periods[len(periods)-1]
		if !at.After(last.LastSeen) {
			return
		}
		if continuous && at.Sub(last.LastSeen) <= locationHistoryMaxGap &&
			last.SystemID == location.SystemID && last.StationID == location.StationID &&
			last.StructureID == location.StructureID && last.CharacterName == location.CharacterName {
			last.LastSeen = at
			last.Observations++
			if location.SystemName != "" {
				last.SystemName = location.SystemName
			}
			return
		}
	}
	history[location.CharacterID] = append(periods, locationPeriod{
		CharacterID: location.CharacterID, CharacterName: location.CharacterName,
		SystemID: location.SystemID, SystemName: location.SystemName,
		StationID: location.StationID, StructureID: location.StructureID,
		FirstSeen: at, LastSeen: at, Observations: 1,
	})
}

func (history locationHistory) at(characterID int64, at time.Time) (locationPeriod, bool) {
	periods := history[characterID]
	index := sort.Search(len(periods), func(index int) bool { return periods[index].FirstSeen.After(at) }) - 1
	if index < 0 || at.After(periods[index].LastSeen.Add(locationHistoryAssumptionWindow)) {
		return locationPeriod{}, false
	}
	return periods[index], true
}

func (history locationHistory) atName(name string, at time.Time) (locationPeriod, bool) {
	var result locationPeriod
	found := false
	for id := range history {
		period, ok := history.at(id, at)
		if !ok || characterKey(period.CharacterName) != characterKey(name) {
			continue
		}
		if found {
			return locationPeriod{}, false
		}
		result, found = period, true
	}
	return result, found
}

func (history locationHistory) clone() locationHistory {
	copy := make(locationHistory, len(history))
	for id, periods := range history {
		copy[id] = append([]locationPeriod(nil), periods...)
	}
	return copy
}

func (history locationHistory) periods() []locationPeriod {
	var periods []locationPeriod
	for _, entries := range history {
		periods = append(periods, entries...)
	}
	sort.Slice(periods, func(i, j int) bool {
		if !periods[i].FirstSeen.Equal(periods[j].FirstSeen) {
			return periods[i].FirstSeen.After(periods[j].FirstSeen)
		}
		return periods[i].CharacterID < periods[j].CharacterID
	})
	return periods
}
