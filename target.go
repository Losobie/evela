package main

import "strings"

const filterTarget = "Target"
const filterTargetType = "Target Type"

func targetGroupingMode(mode string) bool { return mode == filterTarget || mode == filterTargetType }

// The caller removes weapon/hit-quality suffixes first. Preserve names containing
// dashes and use only the explicit hull suffix, without guessing unknown types.
func loggedTarget(detail string) (name, typeName string) {
	name = strings.TrimSpace(detail)
	if match := structureTargetSuffix.FindStringSubmatch(name); match != nil {
		name, typeName = strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
	}
	if strings.HasSuffix(name, "]") {
		if bracket := strings.LastIndexByte(name, '['); bracket >= 0 {
			name = strings.TrimSpace(name[:bracket])
		}
	}
	return strings.Clone(name), strings.Clone(typeName)
}

func damageTarget(detail, weapon, strength string) (string, string) {
	if strength != "" {
		detail, _, _ = cutLastDamageDetail(detail)
	}
	if weapon != "" {
		detail, _, _ = cutLastDamageDetail(detail)
	}
	return loggedTarget(detail)
}

func targetValue(name, typeName, kind string) string {
	if kind == filterTargetType {
		return typeName
	}
	return name
}
