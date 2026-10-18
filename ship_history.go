package main

import (
	"sort"
	"time"
)

const shipHistoryMaxGap = 90 * time.Second

// A period represents sampled observations, not exact boarding/unboarding times.
// FirstSeen and LastSeen bound the evidence; neither end is extrapolated.
type shipPeriod struct {
	CharacterID   int64
	CharacterName string
	ShipID        int64
	TypeID        int64
	TypeName      string
	GroupID       int64
	ClassName     string
	ShipName      string
	FirstSeen     time.Time
	LastSeen      time.Time
	Observations  int
}

// History is independent of saved sign-ins, so disconnecting or replacing tokens
// retains the observations needed to correlate older damage records.
type shipHistory map[int64][]shipPeriod

func (service *esiService) rememberShip(ship shipRecord, continuous bool) {
	if ship.CharacterID <= 0 || characterKey(ship.CharacterName) == "" || ship.TypeID <= 0 || ship.CheckedAt.IsZero() || ship.Error != "" {
		return
	}
	if service.config.ShipHistory == nil {
		service.config.ShipHistory = shipHistory{}
	}
	history := service.config.ShipHistory
	periods := history[ship.CharacterID]
	at := ship.CheckedAt.UTC()
	if len(periods) > 0 {
		last := &periods[len(periods)-1]
		// Ignore duplicate or backwards clock readings to keep periods ordered.
		if !at.After(last.LastSeen) {
			return
		}
		if continuous && at.Sub(last.LastSeen) <= shipHistoryMaxGap &&
			last.ShipID == ship.ShipID && last.TypeID == ship.TypeID &&
			last.ShipName == ship.ShipName && last.CharacterName == ship.CharacterName {
			last.LastSeen = at
			last.Observations++
			if ship.TypeName != "" {
				last.TypeName = ship.TypeName
			}
			if ship.ClassName != "" {
				last.GroupID, last.ClassName = ship.GroupID, ship.ClassName
			}
			return
		}
	}
	history[ship.CharacterID] = append(periods, shipPeriod{
		CharacterID: ship.CharacterID, CharacterName: ship.CharacterName,
		ShipID: ship.ShipID, TypeID: ship.TypeID, TypeName: ship.TypeName, ShipName: ship.ShipName,
		GroupID: ship.GroupID, ClassName: ship.ClassName,
		FirstSeen: at, LastSeen: at, Observations: 1,
	})
}

// Look up a character at the original damage timestamp.
// Missing coverage (including transitions and polling gaps) stays unknown.
func (history shipHistory) at(characterID int64, at time.Time) (shipPeriod, bool) {
	periods := history[characterID]
	index := sort.Search(len(periods), func(index int) bool { return periods[index].FirstSeen.After(at) }) - 1
	if index < 0 || at.After(periods[index].LastSeen) {
		return shipPeriod{}, false
	}
	return periods[index], true
}

// Logs identify characters by name. Ambiguous names are left unknown.
func (history shipHistory) atName(name string, at time.Time) (shipPeriod, bool) {
	var result shipPeriod
	found := false
	for id := range history {
		period, ok := history.at(id, at)
		if !ok || characterKey(period.CharacterName) != characterKey(name) {
			continue
		}
		if found {
			return shipPeriod{}, false
		}
		result, found = period, true
	}
	return result, found
}

func (history shipHistory) clone() shipHistory {
	copy := make(shipHistory, len(history))
	for id, periods := range history {
		copy[id] = append([]shipPeriod(nil), periods...)
	}
	return copy
}

func (history shipHistory) periods() []shipPeriod {
	var periods []shipPeriod
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
