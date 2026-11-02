package main

import "time"

// Immutable name indexes live until the source metadata is refreshed. Values
// derived at a record timestamp are shared across all graphs in one update.
type graphMetadata struct {
	snapshot        *logSnapshot
	attributeValues map[graphAttributes]*graphAttributes
	groupNames      map[string]string
	ships           map[string]shipHistory
	members         map[string][]string
	locations       locationResolver
	characters      map[string]*graphCharacterMetadata
	shipOptions     map[graphShipIdentity][2]damageFilterOption
	systems         map[graphSystemIdentity]damageFilterOption
	damageOptions   map[[2]string]damageFilterOption
	sources         map[damageSource][4]damageFilterOption
}
type graphCharacterMetadata struct {
	key     string
	history shipHistory
	times   map[int64]*graphAttributes
}
type graphAttributes struct{ hull, class, system damageFilterOption }
type graphShipIdentity struct {
	typeID      int64
	groupID     int64
	name, class string
	known       bool
}
type graphSystemIdentity struct {
	id    int64
	name  string
	known bool
}

func (ui *analyzerUI) graphMetadata() *graphMetadata {
	if ui.metadata == nil {
		ui.metadata = &graphMetadata{snapshot: ui.snapshot, attributeValues: map[graphAttributes]*graphAttributes{}, groupNames: map[string]string{}, ships: ui.shipHistory.indexByName(), members: ui.characterFilterGroups(), locations: ui.locationLookup, characters: map[string]*graphCharacterMetadata{}, shipOptions: map[graphShipIdentity][2]damageFilterOption{}, systems: map[graphSystemIdentity]damageFilterOption{}, damageOptions: map[[2]string]damageFilterOption{}, sources: map[damageSource][4]damageFilterOption{}}
		for _, group := range ui.characterGroups {
			ui.metadata.groupNames["group:"+group.ID] = group.Name
		}
	}
	return ui.metadata
}
func (metadata *graphMetadata) character(name string) *graphCharacterMetadata {
	if value := metadata.characters[name]; value != nil {
		return value
	}
	key := characterKey(name)
	value := &graphCharacterMetadata{key: key, history: metadata.ships[key], times: map[int64]*graphAttributes{}}
	metadata.characters[name] = value
	return value
}
func (metadata *graphMetadata) resetTimes() {
	for _, character := range metadata.characters {
		if len(character.times) > 4096 {
			character.times = map[int64]*graphAttributes{}
		} else {
			clear(character.times)
		}
	}
}
func (metadata *graphMetadata) attributes(name string, second int64) graphAttributes {
	character := metadata.character(name)
	if value, ok := character.times[second]; ok {
		return *value
	}
	at := time.Unix(second, 0)
	var ship shipPeriod
	known := false
	for id := range character.history {
		period, ok := character.history.at(id, at)
		if !ok {
			continue
		}
		if known {
			ship = shipPeriod{}
			known = false
			break
		}
		ship, known = period, true
	}
	shipKey := graphShipIdentity{int64(ship.TypeID), int64(ship.GroupID), ship.TypeName, ship.ClassName, known}
	options, ok := metadata.shipOptions[shipKey]
	if !ok {
		options[0], options[1] = shipFilterOptions(ship, known)
		metadata.shipOptions[shipKey] = options
	}
	location, known := metadata.locations.atName(name, at)
	systemKey := graphSystemIdentity{location.SystemID, location.SystemName, known}
	system, ok := metadata.systems[systemKey]
	if !ok {
		system = systemFilterOption(location, known)
		metadata.systems[systemKey] = system
	}
	value := graphAttributes{options[0], options[1], system}
	shared, exists := metadata.attributeValues[value]
	if !exists {
		shared = new(graphAttributes)
		*shared = value
		metadata.attributeValues[value] = shared
	}
	character.times[second] = shared
	return *shared
}
func (metadata *graphMetadata) damageAttribute(hit damageHit, kind string) damageFilterOption {
	value := hit.WeaponType
	if targetGroupingMode(kind) {
		value = targetValue(hit.Target, hit.TargetType, kind)
	}
	if kind == filterWeaponStrength {
		value = hit.WeaponStrength
	}
	key := [2]string{kind, value}
	option, ok := metadata.damageOptions[key]
	if !ok {
		option = namedDamageOption(value, kind)
		metadata.damageOptions[key] = option
	}
	return option
}
func (metadata *graphMetadata) source(source damageSource) [4]damageFilterOption {
	if value, ok := metadata.sources[source]; ok {
		return value
	}
	value := [4]damageFilterOption{sourceFilterOption(source, filterAttacker), sourceFilterOption(source, filterAttackerShip), sourceFilterOption(source, filterNPCNames), sourceFilterOption(source, filterNPCTypes)}
	metadata.sources[source] = value
	return value
}
