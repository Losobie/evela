package parser

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var damageDeflectionSuffix = regexp.MustCompile(`\s+\(([0-9][0-9,.]*) deflected\)$`)

func damageWithoutDeflection(detail string) (string, float64, bool) {
	if !strings.HasSuffix(detail, " deflected)") {
		return detail, 0, false
	}
	match := damageDeflectionSuffix.FindStringSubmatchIndex(detail)
	if match == nil {
		return detail, 0, false
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(detail[match[2]:match[3]], ",", ""), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return detail, 0, false
	}
	detail = strings.TrimSpace(detail[:match[0]])
	// EVE may repeat the hit quality before the parenthesized deflection amount.
	before, quality, ok := cutLastDamageDetail(detail)
	if ok && damageHitQualities[quality] {
		if _, previous, exists := cutLastDamageDetail(before); exists && damageHitQualities[previous] {
			detail = before
		}
	}
	return detail, amount, true
}

func damageWeaponDetails(detail string) (string, string) {
	detail, _, _ = damageWithoutDeflection(detail)
	// Split from the right: pilot and NPC names may themselves contain dashes.
	before, strength, ok := cutLastDamageDetail(detail)
	if !ok {
		return "", ""
	}
	_, weapon, ok := cutLastDamageDetail(before)
	if !ok {
		return "", strength
	}
	return weapon, strength
}

func cutLastDamageDetail(s string) (string, string, bool) {
	i := strings.LastIndex(s, " - ")
	if i < 0 {
		return s, "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+3:]), true
}

func parseDamageMiss(message string) (target, weapon string, incoming, ok bool) {
	body := message
	if i := strings.LastIndex(body, " completely - "); i >= 0 {
		weapon = strings.TrimSpace(body[i+len(" completely - "):])
		body = strings.TrimSpace(body[:i+len(" completely")])
	}
	if strings.HasSuffix(body, " misses you completely") {
		return strings.TrimSuffix(body, " misses you completely"), weapon, true, true
	}
	if strings.HasPrefix(body, "Your ") {
		if i := strings.Index(body, " misses "); i > len("Your ") && strings.HasSuffix(body, " completely") {
			return strings.TrimSuffix(body[i+8:], " completely"), strings.TrimSpace(body[5:i]), false, true
		}
	}
	if strings.HasPrefix(body, "You miss ") && strings.HasSuffix(body, " completely") {
		return strings.TrimSuffix(strings.TrimPrefix(body, "You miss "), " completely"), weapon, false, true
	}
	return "", "", false, false
}
