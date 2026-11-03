package main

import "strings"

const filterTarget = "Target"
const filterTargetType = "Target Type"

func targetGroupingMode(mode string) bool { return mode == filterTarget || mode == filterTargetType }

type combatEntity struct {
	Name, Type, Location, Corporation, CorporationTicker string
}

// Read optional metadata from the right, retaining spaces and dashes in names.
// The caller removes weapon/hit-quality suffixes before parsing the entity.
func parseCombatEntity(detail string) combatEntity {
	entity := combatEntity{}
	name := strings.TrimSpace(detail)
	if strings.HasSuffix(name, ")") {
		if bracket := strings.LastIndexByte(name, '('); bracket >= 0 {
			entity.Type = strings.TrimSpace(name[bracket+1 : len(name)-1])
			name = strings.TrimSpace(name[:bracket])
		}
	}
	for _, field := range []*string{&entity.CorporationTicker, &entity.Corporation} {
		if !strings.HasSuffix(name, "]") {
			break
		}
		bracket := strings.LastIndexByte(name, '[')
		if bracket < 0 {
			break
		}
		*field = strings.TrimSpace(name[bracket+1 : len(name)-1])
		name = strings.TrimSpace(name[:bracket])
	}
	if strings.HasSuffix(name, ")") {
		if bracket := strings.LastIndex(name, " ("); bracket >= 0 {
			entity.Location = strings.TrimSpace(name[bracket+2 : len(name)-1])
			name = strings.TrimSpace(name[:bracket])
		}
	}
	entity.Name = strings.Clone(name)
	entity.Type, entity.Location = strings.Clone(entity.Type), strings.Clone(entity.Location)
	entity.Corporation, entity.CorporationTicker = strings.Clone(entity.Corporation), strings.Clone(entity.CorporationTicker)
	return entity
}

func loggedTarget(detail string) (string, string) {
	// Remote-assistance messages may append a module after the entity suffix.
	if i := strings.LastIndex(detail, ") - "); i >= 0 {
		detail = detail[:i+1]
	}
	entity := parseCombatEntity(detail)
	return entity.Name, entity.Type
}

func damageTarget(detail, weapon, strength string) combatEntity {
	detail, _, _ = damageWithoutDeflection(detail)
	if strength != "" {
		detail, _, _ = cutLastDamageDetail(detail)
	}
	if weapon != "" {
		detail, _, _ = cutLastDamageDetail(detail)
	}
	return parseCombatEntity(detail)
}

func (hit *damageHit) setTarget(entity combatEntity) {
	hit.Target, hit.TargetType, hit.TargetLocation = entity.Name, entity.Type, entity.Location
	hit.TargetCorporation, hit.TargetCorporationTicker = entity.Corporation, entity.CorporationTicker
}

func targetValue(name, typeName, kind string) string {
	if kind == filterTargetType {
		return typeName
	}
	return name
}
