package main

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

const wormholeJumpGrace = 30 * time.Second
const inferredLogLocationSource = "Game log (assumed)"

// A log session supplies continuous coverage between timestamped entries. Never
// carry its last jump across an unobserved session or an arbitrary offline gap.
type logLocationSession struct {
	CharacterName string
	Start, End    int64
	Events        []logLocationEvent
}

type logLocationHistory map[string]logLocationSession

func (snapshot *logSnapshot) locationSessions() logLocationHistory {
	if snapshot.LocationSessions != nil {
		return snapshot.LocationSessions
	}
	history := logLocationHistory{}
	for _, log := range snapshot.Logs {
		if log.Character == "" || len(log.LocationEvents) == 0 || len(log.EntrySeconds) == 0 {
			continue
		}
		start, end := log.EntrySeconds[0], log.EntrySeconds[len(log.EntrySeconds)-1]
		if !log.SessionStarted.IsZero() && log.SessionStarted.Unix() <= start {
			start = log.SessionStarted.Unix()
		}
		key := fmt.Sprintf("%s:%d", characterKey(log.Character), start)
		history.merge(logLocationHistory{key: {CharacterName: log.Character, Start: start, End: end, Events: log.LocationEvents}})
	}
	return history
}

func (history logLocationHistory) merge(other logLocationHistory) bool {
	changed := false
	for key, session := range other {
		previous, exists := history[key]
		if exists && previous.Start <= session.Start && previous.End >= session.End && slices.Equal(previous.Events, session.Events) {
			continue
		}
		if exists {
			session.Start, session.End = min(previous.Start, session.Start), max(previous.End, session.End)
			session.Events = append(append([]logLocationEvent(nil), previous.Events...), session.Events...)
		} else {
			session.Events = append([]logLocationEvent(nil), session.Events...)
		}
		sort.Slice(session.Events, func(i, j int) bool {
			a, b := session.Events[i], session.Events[j]
			if a.Second != b.Second {
				return a.Second < b.Second
			}
			if a.To != b.To {
				return a.To < b.To
			}
			if a.From != b.From {
				return a.From < b.From
			}
			return !a.Inferred && b.Inferred
		})
		session.Events = slices.Compact(session.Events)
		if !exists || previous.Start != session.Start || previous.End != session.End || !slices.Equal(previous.Events, session.Events) {
			history[key], changed = session, true
		}
	}
	return changed
}

func (history logLocationHistory) clone() logLocationHistory {
	copy := logLocationHistory{}
	for key, session := range history {
		session.Events = append([]logLocationEvent(nil), session.Events...)
		copy[key] = session
	}
	return copy
}

// A period's LastSeen is its coverage end. These intervals are derived from
// movement messages and session bounds, rather than sampled ESI observations.
func (history logLocationHistory) periods() []locationPeriod {
	var result []locationPeriod
	// Apply the same recent-jump suppression as the resolver. This viewer shows
	// log evidence; an assumed period can still be superseded by ESI at lookup.
	for _, index := range newLocationResolver(nil, history).logs {
		for _, session := range index.sessions {
			if len(session.Events) == 0 {
				continue
			}
			appendPeriod := func(name string, first, last int64, inferred bool) {
				if name != "" && last >= first {
					period := locationPeriod{CharacterName: session.CharacterName, SystemName: name,
						FirstSeen: time.Unix(first, 0).UTC(), LastSeen: time.Unix(last, 0).UTC(), Observations: 1}
					if inferred {
						period.Source = inferredLogLocationSource
					}
					result = append(result, period)
				}
			}
			appendPeriod(session.Events[0].From, session.Start, session.Events[0].Second-1, false)
			for i, event := range session.Events {
				end := session.End
				if i+1 < len(session.Events) {
					end = min(end, session.Events[i+1].Second-1)
				}
				appendPeriod(event.To, event.Second, end, event.Inferred)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].FirstSeen.Equal(result[j].FirstSeen) {
			return result[i].FirstSeen.After(result[j].FirstSeen)
		}
		return result[i].CharacterName < result[j].CharacterName
	})
	return result
}

type locationResolver struct {
	esi     map[string]locationHistory
	logs    map[string]*logLocationIndex
	systems map[string]int64
}

type logLocationIndex struct {
	sessions []logLocationSession
	maxEnd   []int64
}

func newLocationResolver(esi locationHistory, logs logLocationHistory) locationResolver {
	resolver := locationResolver{esi: esi.indexByName(), logs: map[string]*logLocationIndex{}, systems: map[string]int64{}}
	for _, periods := range esi {
		for _, period := range periods {
			if period.SystemID > 0 && period.SystemName != "" {
				resolver.systems[characterKey(period.SystemName)] = period.SystemID
			}
		}
	}
	for _, session := range logs {
		key := characterKey(session.CharacterName)
		if resolver.logs[key] == nil {
			resolver.logs[key] = &logLocationIndex{}
		}
		resolver.logs[key].sessions = append(resolver.logs[key].sessions, session)
	}
	for _, index := range resolver.logs {
		var jumps []int64
		for _, session := range index.sessions {
			for _, event := range session.Events {
				if !event.Inferred {
					jumps = append(jumps, event.Second)
				}
			}
		}
		slices.Sort(jumps)
		jumps = slices.Compact(jumps)
		for i := range index.sessions {
			index.sessions[i].Events = selectRecords(index.sessions[i].Events, func(event logLocationEvent) bool {
				if !event.Inferred {
					return true
				}
				previous := sort.Search(len(jumps), func(i int) bool { return jumps[i] > event.Second }) - 1
				return previous < 0 || event.Second-jumps[previous] > int64(wormholeJumpGrace/time.Second)
			})
		}
		sort.Slice(index.sessions, func(i, j int) bool { return index.sessions[i].Start < index.sessions[j].Start })
		var end int64
		for i, session := range index.sessions {
			if i == 0 || session.End > end {
				end = session.End
			}
			index.maxEnd = append(index.maxEnd, end)
		}
	}
	return resolver
}

func (resolver locationResolver) atName(name string, at time.Time) (locationPeriod, bool) {
	second := at.Unix()
	var system string
	var latest int64
	found := false
	inferred := false
	logs := resolver.logs[characterKey(name)]
	if logs == nil {
		return resolver.esi[characterKey(name)].atName(name, at)
	}
	last := sort.Search(len(logs.sessions), func(i int) bool { return logs.sessions[i].Start > second }) - 1
	for i := last; i >= 0 && logs.maxEnd[i]+int64(locationHistoryAssumptionWindow/time.Second) >= second; i-- {
		session := logs.sessions[i]
		if second < session.Start || second > session.End+int64(locationHistoryAssumptionWindow/time.Second) || len(session.Events) == 0 {
			continue
		}
		index := sort.Search(len(session.Events), func(i int) bool { return session.Events[i].Second > second }) - 1
		candidate, established := session.Events[0].From, session.Start
		assumed := false
		if index >= 0 {
			candidate, established = session.Events[index].To, session.Events[index].Second
			assumed = session.Events[index].Inferred
			// Conflicting exports at the same timestamp must not pick a system
			// based on map order or fall back to a cached ESI response.
			for previous := index - 1; previous >= 0 && session.Events[previous].Second == established; previous-- {
				if characterKey(session.Events[previous].To) != characterKey(session.Events[index].To) {
					candidate = ""
				}
			}
		}
		if candidate == "" && index < 0 {
			continue
		}
		if !found || established > latest {
			system, latest, found = candidate, established, true
			inferred = assumed
		} else if established == latest {
			if inferred && !assumed {
				system, inferred = candidate, false
			} else if inferred == assumed && characterKey(system) != characterKey(candidate) {
				system = ""
			}
		}
	}
	if found {
		if inferred && system != "" {
			if location, ok := resolver.esi[characterKey(name)].atName(name, at); ok && (location.SystemID > 0 || location.SystemName != "") {
				return location, true
			}
			return locationPeriod{CharacterName: name, SystemName: system, Source: inferredLogLocationSource}, true
		}
		return locationPeriod{CharacterName: name, SystemName: system, SystemID: resolver.systems[characterKey(system)], Source: "Game log"}, system != ""
	}
	return resolver.esi[characterKey(name)].atName(name, at)
}

func (ui *analyzerUI) resolvedLocations() locationResolver {
	// Resolution is read-only. Merge allocates new event slices when needed,
	// so unchanged saved sessions can be shared instead of copied on every zoom.
	logs := logLocationHistory{}
	for key, session := range ui.logLocations {
		logs[key] = session
	}
	logs.merge(ui.snapshot.locationSessions())
	return newLocationResolver(ui.locationHistory, logs)
}

func (ui *analyzerUI) rememberLogLocations(locations logLocationHistory) {
	if ui.logLocations == nil {
		ui.logLocations = logLocationHistory{}
	}
	if !ui.logLocations.merge(locations) {
		return
	}
	// Keep only the latest complete history while the ESI worker is busy.
	select {
	case <-ui.logLocationUpdates:
	default:
	}
	ui.logLocationUpdates <- ui.logLocations.clone()
}
