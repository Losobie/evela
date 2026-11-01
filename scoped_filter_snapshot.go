package main

import "time"

// Local filters intersect global filters at the record timestamp, before buckets,
// rolling averages, totals, and individual-hit plots are built.
func (ui *analyzerUI) filteredDamageSnapshot(snapshot *logSnapshot) *logSnapshot {
	restricted := false
	for scope, kinds := range ui.visibleFilters {
		for kind, visible := range kinds {
			if visible && ui.storedScopeFilter(scope, kind).restricted() {
				restricted = true
			}
		}
	}
	if !restricted {
		return nil
	}
	ships, members := ui.shipHistory.indexByName(), ui.characterFilterGroups()
	matches := func(scope string, log *parsedLog, second int64) bool {
		at := time.Unix(second, 0)
		for _, filterScope := range []string{globalFilterScope, scope} {
			if filterScope != globalFilterScope && !ui.scopeFilter(filterScope, filterCharacters).selected(log.Character) {
				return false
			}
			if !characterGroupsSelected(ui.scopeFilter(filterScope, filterCharacterGroup), members[characterKey(log.Character)]) {
				return false
			}
			if ui.scopeFilter(filterScope, filterSystem).restricted() {
				location, known := ui.locationLookup.atName(log.Character, at)
				if !ui.scopeFilter(filterScope, filterSystem).selected(systemFilterOption(location, known).Key) {
					return false
				}
			}
			if ui.scopeFilter(filterScope, filterShipType).restricted() || ui.scopeFilter(filterScope, filterShipClass).restricted() {
				ship, known := ships[characterKey(log.Character)].atName(log.Character, at)
				hull, class := shipFilterOptions(ship, known)
				if !ui.scopeFilter(filterScope, filterShipType).selected(hull.Key) || !ui.scopeFilter(filterScope, filterShipClass).selected(class.Key) {
					return false
				}
			}
		}
		return true
	}
	sourceMatches := func(log *parsedLog, sourceID int) bool {
		source := damageSource{}
		if sourceID >= 0 && sourceID < len(log.Sources) {
			source = log.Sources[sourceID]
		}
		for _, kind := range []string{filterNPCNames, filterNPCTypes, filterAttacker, filterAttackerShip} {
			if !ui.scopeFilter(graphIncoming, kind).selected(sourceFilterOption(source, kind).Key) {
				return false
			}
		}
		return true
	}
	weaponMatches := func(scope string, hit damageHit) bool {
		for _, kind := range []string{filterWeaponType, filterWeaponStrength} {
			if !ui.scopeFilter(scope, kind).selected(hitWeaponOption(hit, kind).Key) {
				return false
			}
		}
		return true
	}
	filtered := *snapshot
	filtered.Logs = nil
	for _, log := range snapshot.Logs {
		copyLog := *log
		copyLog.Hits, copyLog.Points, copyLog.IncomingSources = nil, nil, nil
		copyLog.EWar, copyLog.Logistics, copyLog.Mining, copyLog.StructureEvents = nil, nil, nil, nil
		for _, hit := range log.Hits {
			scope := graphOutgoing
			if hit.Incoming {
				scope = graphIncoming
			}
			if matches(scope, log, hit.Second) && weaponMatches(scope, hit) && (!hit.Incoming || sourceMatches(log, hit.Source)) {
				copyLog.Hits = append(copyLog.Hits, hit)
			}
		}
		if len(log.Hits) > 0 {
			rebuildDamageFromHits(&copyLog)
		} else {
			incoming := map[int64]incomingSourcePoint{}
			for _, point := range log.IncomingSources {
				if matches(graphIncoming, log, point.Second) && sourceMatches(log, point.Source) && weaponMatches(graphIncoming, damageHit{}) {
					copyLog.IncomingSources = append(copyLog.IncomingSources, point)
					value := incoming[point.Second]
					value.Damage += point.Damage
					value.Hits += point.Hits
					incoming[point.Second] = value
				}
			}
			for _, point := range log.Points {
				if !matches(graphOutgoing, log, point.Second) || !weaponMatches(graphOutgoing, damageHit{}) {
					point.Outgoing, point.OutgoingHits = 0, 0
				}
				if !matches(graphIncoming, log, point.Second) || !weaponMatches(graphIncoming, damageHit{}) {
					point.Incoming, point.IncomingHits = 0, 0
				} else if len(log.IncomingSources) > 0 {
					value := incoming[point.Second]
					point.Incoming, point.IncomingHits = value.Damage, value.Hits
				} else if !sourceMatches(log, -1) {
					point.Incoming, point.IncomingHits = 0, 0
				}
				copyLog.Points = append(copyLog.Points, point)
			}
		}
		for _, event := range log.EWar {
			if matches(graphIncomingEWar, log, event.Second) && ui.scopeFilter(graphIncomingEWar, filterEffectType).selected(event.Effect) {
				copyLog.EWar = append(copyLog.EWar, event)
			}
		}
		for _, event := range log.Logistics {
			scope := graphOutgoingLogistics
			if event.Incoming {
				scope = graphIncomingLogistics
			}
			if matches(scope, log, event.Second) {
				copyLog.Logistics = append(copyLog.Logistics, event)
			}
		}
		for _, event := range log.Mining {
			if matches(graphMining, log, event.Second) {
				copyLog.Mining = append(copyLog.Mining, event)
			}
		}
		for _, event := range log.StructureEvents {
			if matches(graphOutgoing, log, event.Second) {
				copyLog.StructureEvents = append(copyLog.StructureEvents, event)
			}
		}
		filtered.Logs = append(filtered.Logs, &copyLog)
	}
	return &filtered
}
