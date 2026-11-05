package main

import (
	"regexp"
	"strings"
)

const filterTarget = "Target"
const filterTargetType = "Target Type"

func targetGroupingMode(mode string) bool { return mode == filterTarget || mode == filterTargetType }

type combatEntity struct {
	Name, Type, Location, Corporation, CorporationTicker string
}

// Customs offices identify their type by name and put a planet / location in
// parentheses, followed by the owner's full corporation name (not a ticker).
var locatedCustomsOffice = regexp.MustCompile(`(?i)^(Customs Office(?: Gantry)?) \(([^()]+)\)(?:\s*\[([^\[\]]+)\])?(?:\[([^\[\]]+)\])?(?:\((Customs Office(?: Gantry)?)\))?$`)

// Read optional metadata from the right, retaining spaces and dashes in names.
// The caller removes weapon/hit-quality suffixes before parsing the entity.
func parseCombatEntity(detail string) combatEntity {
	entity := combatEntity{}
	name := strings.TrimSpace(detail)
	if len(name) >= len("Customs Office ") && strings.EqualFold(name[:len("Customs Office ")], "Customs Office ") {
		if match := locatedCustomsOffice.FindStringSubmatch(name); match != nil {
			typeName := "Customs Office"
			if strings.EqualFold(match[1], "Customs Office Gantry") {
				typeName += " Gantry"
			}
			return combatEntity{Name: typeName, Type: typeName, Location: strings.Clone(strings.TrimSpace(match[2])),
				Corporation: strings.Clone(strings.TrimSpace(match[3])), CorporationTicker: strings.Clone(strings.TrimSpace(match[4]))}
		}
	}
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
	if entity.Type == "" {
		// Some structures / NPC-owned hulls omit the usual (type) suffix.
		// Use only an exact SDE name match; explicit logged types take priority.
		entity.Type = loadHullClasses()[characterKey(entity.Name)]
	}
	entity.Type, entity.Location = strings.Clone(entity.Type), strings.Clone(entity.Location)
	entity.Corporation, entity.CorporationTicker = strings.Clone(entity.Corporation), strings.Clone(entity.CorporationTicker)
	return entity
}

func loggedTarget(detail string) (string, string) {
	entity := logisticsTarget(detail)
	return entity.Name, entity.Type
}

func logisticsTarget(detail string) combatEntity {
	entity, _ := logisticsTargetDetails(detail)
	return entity
}

// Remote assistance can put the hull before alliance, corporation and pilot
// brackets (alliance is optional), followed by a module with either a single or
// double dash separator. Parentheses distinguish structure locations / hulls.
var logisticsShipTarget = regexp.MustCompile(`^([^()\[\]]+?)\s+(?:\[([^\[\]]*)\]\s*)?\[([^\[\]]*)\]\s*\[([^\[\]]+)\](?:\s+-\s+.*)?$`)

func logisticsTargetDetails(detail string) (combatEntity, string) {
	if match := logisticsShipTarget.FindStringSubmatch(strings.TrimSpace(detail)); match != nil {
		return combatEntity{Name: strings.Clone(strings.TrimSpace(match[4])), Type: strings.Clone(strings.TrimSpace(match[1])),
			CorporationTicker: strings.Clone(strings.TrimSpace(match[3]))}, strings.Clone(strings.TrimSpace(match[2]))
	}
	// Remote-assistance messages may append a module after the entity suffix.
	if i := strings.LastIndex(detail, " - "); i >= 0 {
		entity := strings.TrimSpace(detail[:i])
		if strings.HasSuffix(entity, ")") || strings.HasSuffix(entity, "]") || loadHullClasses()[characterKey(entity)] != "" {
			detail = entity
		}
	}
	return parseCombatEntity(detail), ""
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
