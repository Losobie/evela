package main

import "sort"

type graphFilterCatalog struct {
	values     map[string]map[string]string
	metadata   *graphMetadata
	characters map[string]bool
	weapons    map[[2]string]bool
	sources    map[damageSource]bool
	targets    map[[2]string]bool
}

func newGraphFilterCatalog(metadata *graphMetadata, kinds []string) *graphFilterCatalog {
	catalog := &graphFilterCatalog{values: map[string]map[string]string{}, metadata: metadata, characters: map[string]bool{}, weapons: map[[2]string]bool{}, sources: map[damageSource]bool{}, targets: map[[2]string]bool{}}
	for _, kind := range kinds {
		catalog.values[kind] = map[string]string{}
	}
	return catalog
}
func (catalog *graphFilterCatalog) add(kind string, option damageFilterOption) {
	if values := catalog.values[kind]; values != nil {
		values[option.Key] = option.Label
	}
}
func (catalog *graphFilterCatalog) common(character string, second int64) {
	if !catalog.characters[character] {
		catalog.characters[character] = true
		catalog.add(filterCharacters, damageFilterOption{Key: character, Label: character})
		groups := catalog.metadata.members[catalog.metadata.character(character).key]
		if len(groups) == 0 {
			catalog.add(filterCharacterGroup, damageFilterOption{Key: filterUngrouped, Label: ungroupedCharacters})
		}
		for _, key := range groups {
			catalog.add(filterCharacterGroup, damageFilterOption{Key: key, Label: catalog.metadata.groupNames[key]})
		}
	}
	attributes := catalog.metadata.attributes(character, second)
	catalog.add(filterShipType, attributes.hull)
	catalog.add(filterShipClass, attributes.class)
	catalog.add(filterSystem, attributes.system)
}
func (catalog *graphFilterCatalog) damage(hit damageHit, source damageSource, incoming bool) {
	if !incoming {
		catalog.target(hit.Target, hit.TargetType)
	}
	key := [2]string{hit.WeaponType, hit.WeaponStrength}
	if !catalog.weapons[key] {
		catalog.weapons[key] = true
		catalog.add(filterWeaponType, catalog.metadata.damageAttribute(hit, filterWeaponType))
		catalog.add(filterWeaponStrength, catalog.metadata.damageAttribute(hit, filterWeaponStrength))
	}
	if incoming && !catalog.sources[source] {
		catalog.sources[source] = true
		options := catalog.metadata.source(source)
		for index, kind := range []string{filterAttacker, filterAttackerShip, filterNPCNames, filterNPCTypes} {
			catalog.add(kind, options[index])
		}
	}
}
func (catalog *graphFilterCatalog) target(name, typeName string) {
	key := [2]string{name, typeName}
	if catalog.targets[key] {
		return
	}
	catalog.targets[key] = true
	hit := damageHit{Target: name, TargetType: typeName}
	for _, kind := range []string{filterTarget, filterTargetType} {
		catalog.add(kind, catalog.metadata.damageAttribute(hit, kind))
	}
}
func (catalog *graphFilterCatalog) options() map[string][]damageFilterOption {
	result := map[string][]damageFilterOption{}
	for kind, values := range catalog.values {
		for key, label := range values {
			result[kind] = append(result[kind], damageFilterOption{Key: key, Label: label})
		}
		sort.Slice(result[kind], func(i, j int) bool { return compareTotalsText(result[kind][i].Label, result[kind][j].Label) < 0 })
	}
	return result
}

func (ui *analyzerUI) buildScopeFilterOptions(scope string, snapshot *logSnapshot) map[string][]damageFilterOption {
	catalog := newGraphFilterCatalog(ui.graphMetadata(), availableFilters(scope))
	for _, log := range snapshot.Logs {
		// Input records are sorted, so resolve common metadata once per second in
		// each stream. The metadata cache shares the result with the other graphs.
		register := func(second int64) { catalog.common(log.Character, second) }
		switch scope {
		case graphOutgoing, graphIncoming:
			incoming := scope == graphIncoming
			var last int64
			haveLast := false
			for _, hit := range log.Hits {
				if hit.Incoming != incoming {
					continue
				}
				if !haveLast || last != hit.Second {
					register(hit.Second)
					last = hit.Second
					haveLast = true
				}
				source := damageSource{}
				if incoming && hit.Source >= 0 && hit.Source < len(log.Sources) {
					source = log.Sources[hit.Source]
				}
				catalog.damage(hit, source, incoming)
			}
			if len(log.Hits) == 0 {
				for _, point := range log.Points {
					if incoming && (point.Incoming != 0 || point.IncomingHits != 0) || !incoming && (point.Outgoing != 0 || point.OutgoingHits != 0) {
						register(point.Second)
						catalog.damage(damageHit{}, damageSource{}, incoming)
					}
				}
			}
		case graphIncomingEWar:
			for _, event := range log.EWar {
				register(event.Second)
				catalog.add(filterEffectType, damageFilterOption{Key: event.Effect, Label: event.Effect})
			}
		case graphMining:
			for _, event := range log.Mining {
				register(event.Second)
				catalog.add(filterOreName, damageFilterOption{Key: event.Ore, Label: event.Ore})
				kind := oreType(event.Ore)
				catalog.add(filterOreType, damageFilterOption{Key: kind, Label: kind})
				catalog.add(filterMiningMessage, damageFilterOption{Key: event.Kind, Label: event.Kind})
			}
		case graphIncomingLogistics, graphOutgoingLogistics:
			for _, event := range log.Logistics {
				if event.Incoming == (scope == graphIncomingLogistics) {
					register(event.Second)
					if !event.Incoming {
						catalog.target(event.Target, event.TargetType)
					}
				}
			}
		}
	}
	result := catalog.options()
	if scope == graphIncomingLogistics || scope == graphOutgoingLogistics {
		result[filterLogisticsType] = logisticsTypeOptions
	}
	return result
}
