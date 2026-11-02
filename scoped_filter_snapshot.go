package main

// Keep the immutable original slice when every record matches. Allocate only
// once a restriction actually excludes a record; never compact shared storage.
func selectRecords[T any](records []T, keep func(T) bool) []T {
	for index, record := range records {
		if keep(record) {
			continue
		}
		result := make([]T, 0, len(records))
		result = append(result, records[:index]...)
		for _, remaining := range records[index+1:] {
			if keep(remaining) {
				result = append(result, remaining)
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	}
	return records
}
func sameRecords[T any](a, b []T) bool { return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0]) }

type graphRecordFilter struct {
	characters, groups, shipType, shipClass, system *damageFilterSelection
	weaponType, weaponStrength, effect              *damageFilterSelection
	target, targetType                              *damageFilterSelection
	sources                                         [4]*damageFilterSelection
}

func (ui *analyzerUI) compileRecordFilter(scope string) graphRecordFilter {
	active := func(kind string) *damageFilterSelection {
		selection := ui.scopeFilter(scope, kind)
		if selection.restricted() {
			return selection
		}
		return nil
	}
	filter := graphRecordFilter{groups: active(filterCharacterGroup), shipType: active(filterShipType), shipClass: active(filterShipClass), system: active(filterSystem)}
	if scope != globalFilterScope {
		filter.characters = active(filterCharacters)
	}
	if scope == graphOutgoing || scope == graphIncoming {
		filter.weaponType = active(filterWeaponType)
		filter.weaponStrength = active(filterWeaponStrength)
	}
	if scope == graphOutgoing || scope == graphOutgoingLogistics {
		filter.target, filter.targetType = active(filterTarget), active(filterTargetType)
	}
	if scope == graphIncoming {
		for index, kind := range []string{filterAttacker, filterAttackerShip, filterNPCNames, filterNPCTypes} {
			filter.sources[index] = active(kind)
		}
	}
	if scope == graphIncomingEWar {
		filter.effect = active(filterEffectType)
	}
	return filter
}
func (filter graphRecordFilter) commonActive() bool {
	return filter.characters != nil || filter.groups != nil || filter.shipType != nil || filter.shipClass != nil || filter.system != nil
}
func (filter graphRecordFilter) active() bool {
	return filter.commonActive() || filter.target != nil || filter.targetType != nil || filter.weaponType != nil || filter.weaponStrength != nil || filter.effect != nil || filter.sources != [4]*damageFilterSelection{}
}
func (filter graphRecordFilter) characterAllowed(metadata *graphMetadata, name string) bool {
	if !filter.characters.selected(name) {
		return false
	}
	if filter.groups != nil && !characterGroupsSelected(filter.groups, metadata.members[metadata.character(name).key]) {
		return false
	}
	return true
}
func (filter graphRecordFilter) timeAllowed(metadata *graphMetadata, name string, second int64) bool {
	if filter.shipType == nil && filter.shipClass == nil && filter.system == nil {
		return true
	}
	attributes := metadata.attributes(name, second)
	return filter.shipType.selected(attributes.hull.Key) && filter.shipClass.selected(attributes.class.Key) && filter.system.selected(attributes.system.Key)
}
func (filter graphRecordFilter) damageAllowed(metadata *graphMetadata, hit damageHit) bool {
	return (filter.weaponType == nil || filter.weaponType.selected(metadata.damageAttribute(hit, filterWeaponType).Key)) && (filter.weaponStrength == nil || filter.weaponStrength.selected(metadata.damageAttribute(hit, filterWeaponStrength).Key)) && filter.targetAllowed(metadata, hit.Target, hit.TargetType)
}
func (filter graphRecordFilter) targetAllowed(metadata *graphMetadata, name, typeName string) bool {
	hit := damageHit{Target: name, TargetType: typeName}
	return (filter.target == nil || filter.target.selected(metadata.damageAttribute(hit, filterTarget).Key)) && (filter.targetType == nil || filter.targetType.selected(metadata.damageAttribute(hit, filterTargetType).Key))
}
func (filter graphRecordFilter) sourceAllowed(metadata *graphMetadata, log *parsedLog, id int) bool {
	if filter.sources == [4]*damageFilterSelection{} {
		return true
	}
	source := damageSource{}
	if id >= 0 && id < len(log.Sources) {
		source = log.Sources[id]
	}
	options := metadata.source(source)
	for index, selection := range filter.sources {
		if !selection.selected(options[index].Key) {
			return false
		}
	}
	return true
}

func (ui *analyzerUI) filteredDamageSnapshot(snapshot *logSnapshot) *logSnapshot {
	global := ui.compileRecordFilter(globalFilterScope)
	plans := map[string]graphRecordFilter{}
	restricted := global.active()
	for _, graph := range graphVisibilityOptions {
		plan := ui.compileRecordFilter(graph.id)
		plans[graph.id] = plan
		restricted = restricted || plan.active()
	}
	if !restricted {
		return nil
	}
	metadata := ui.graphMetadata()
	filtered := *snapshot
	filtered.Logs = nil
	for _, log := range snapshot.Logs {
		copyLog := *log
		common := map[string]func(int64) bool{}
		for scope, plan := range plans {
			plan := plan
			allowed := global.characterAllowed(metadata, log.Character) && plan.characterAllowed(metadata, log.Character)
			common[scope] = func(second int64) bool {
				return allowed && global.timeAllowed(metadata, log.Character, second) && plan.timeAllowed(metadata, log.Character, second)
			}
		}
		incoming, outgoing := plans[graphIncoming], plans[graphOutgoing]
		if global.active() || incoming.active() || outgoing.active() {
			if len(log.Hits) > 0 {
				copyLog.Hits = selectRecords(log.Hits, func(hit damageHit) bool {
					scope, plan := graphOutgoing, outgoing
					if hit.Incoming {
						scope, plan = graphIncoming, incoming
					}
					if !global.active() && !plan.active() {
						return true
					}
					return common[scope](hit.Second) && plan.damageAllowed(metadata, hit) && (!hit.Incoming || plan.sourceAllowed(metadata, log, hit.Source))
				})
				if !sameRecords(log.Hits, copyLog.Hits) {
					rebuildDamageFromHits(&copyLog)
				}
			} else {
				// Legacy/synthetic summaries have no raw hits. Preserve their existing
				// attribution rules, without treating an unknown source as a known NPC.
				copyLog.IncomingSources = selectRecords(log.IncomingSources, func(point incomingSourcePoint) bool {
					return common[graphIncoming](point.Second) && incoming.damageAllowed(metadata, damageHit{}) && incoming.sourceAllowed(metadata, log, point.Source)
				})
				values := map[int64]incomingSourcePoint{}
				for _, point := range copyLog.IncomingSources {
					value := values[point.Second]
					value.Damage += point.Damage
					value.Hits += point.Hits
					values[point.Second] = value
				}
				copyLog.Points = make([]damagePoint, 0, len(log.Points))
				for _, point := range log.Points {
					if !common[graphOutgoing](point.Second) || !outgoing.damageAllowed(metadata, damageHit{}) {
						point.Outgoing, point.OutgoingHits = 0, 0
					}
					if !common[graphIncoming](point.Second) || !incoming.damageAllowed(metadata, damageHit{}) {
						point.Incoming, point.IncomingHits = 0, 0
					} else if len(log.IncomingSources) > 0 {
						value := values[point.Second]
						point.Incoming, point.IncomingHits = value.Damage, value.Hits
					} else if !incoming.sourceAllowed(metadata, log, -1) {
						point.Incoming, point.IncomingHits = 0, 0
					}
					copyLog.Points = append(copyLog.Points, point)
				}
			}
		}
		if plan := plans[graphIncomingEWar]; global.commonActive() || plan.active() {
			copyLog.EWar = selectRecords(log.EWar, func(event ewarEvent) bool {
				return common[graphIncomingEWar](event.Second) && plan.effect.selected(event.Effect)
			})
		}
		if global.commonActive() || plans[graphIncomingLogistics].commonActive() || plans[graphOutgoingLogistics].active() {
			copyLog.Logistics = selectRecords(log.Logistics, func(event logisticsEvent) bool {
				scope := graphOutgoingLogistics
				if event.Incoming {
					scope = graphIncomingLogistics
				}
				return common[scope](event.Second) && plans[scope].targetAllowed(metadata, event.Target, event.TargetType)
			})
		}
		if global.commonActive() || plans[graphMining].commonActive() {
			copyLog.Mining = selectRecords(log.Mining, func(event miningEvent) bool { return common[graphMining](event.Second) })
		}
		if global.commonActive() || outgoing.commonActive() || outgoing.target != nil || outgoing.targetType != nil {
			copyLog.StructureEvents = selectRecords(log.StructureEvents, func(event structureEvent) bool {
				if !common[graphOutgoing](event.Second) {
					return false
				}
				if outgoing.target == nil && outgoing.targetType == nil {
					return true
				}
				name, typeName := loggedTarget(event.Target)
				return outgoing.targetAllowed(metadata, name, typeName)
			})
		}
		filtered.Logs = append(filtered.Logs, &copyLog)
	}
	return &filtered
}
