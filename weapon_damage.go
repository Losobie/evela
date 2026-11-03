package main

import (
	"math"
	"regexp"
	"sort"
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

const (
	filterWeaponType     = "Weapon Type"
	filterWeaponStrength = "Weapon Strength"
)

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

func damageAttributeGroupingMode(mode string) bool {
	return mode == filterWeaponType || mode == filterWeaponStrength || targetGroupingMode(mode)
}
func namedDamageOption(value, kind string) damageFilterOption {
	if value == "" {
		return damageFilterOption{Key: "unknown", Label: "Unknown " + strings.ToLower(kind)}
	}
	return damageFilterOption{Key: "name:" + value, Label: value}
}
func hitDamageAttributeOption(hit damageHit, mode string) damageFilterOption {
	if targetGroupingMode(mode) {
		return namedDamageOption(targetValue(hit.Target, hit.TargetType, mode), mode)
	}
	if mode == filterWeaponStrength {
		return namedDamageOption(hit.WeaponStrength, mode)
	}
	return namedDamageOption(hit.WeaponType, mode)
}
func (ui *analyzerUI) damageAttributeGrouping() bool {
	return ui.groupSelect != nil && damageAttributeGroupingMode(ui.groupSelect.Selected)
}

func rebuildDamageFromHits(log *parsedLog) {
	points := make([]damagePoint, 0, min(len(log.Points), len(log.Hits)))
	sources := make([]incomingSourcePoint, 0, min(len(log.IncomingSources), len(log.Hits)))
	sourceIDs := map[int]int{}
	sourceStart := 0
	flush := func() {
		if len(sources)-sourceStart < 2 {
			return
		}
		sort.Slice(sources[sourceStart:], func(i, j int) bool { return sources[sourceStart+i].Source < sources[sourceStart+j].Source })
	}
	for _, hit := range log.Hits {
		if hit.WeaponStrength == "Misses" {
			continue
		}
		if len(points) == 0 || points[len(points)-1].Second != hit.Second {
			flush()
			sourceStart = len(sources)
			clear(sourceIDs)
			points = append(points, damagePoint{Second: hit.Second})
		}
		point := &points[len(points)-1]
		if hit.Incoming {
			point.Incoming += hit.Amount
			point.IncomingHits++
			if hit.Source >= 0 {
				index, exists := sourceIDs[hit.Source]
				if !exists {
					index = len(sources)
					sources = append(sources, incomingSourcePoint{Second: hit.Second, Source: hit.Source})
					sourceIDs[hit.Source] = index
				}
				sources[index].Damage += hit.Amount
				sources[index].Hits++
			}
		} else {
			point.Outgoing += hit.Amount
			point.OutgoingHits++
		}
	}
	flush()
	log.Points, log.IncomingSources = points, sources
}

func sortDamageRecords(log *parsedLog) {
	sort.Slice(log.Points, func(i, j int) bool { return log.Points[i].Second < log.Points[j].Second })
	sort.Slice(log.IncomingSources, func(i, j int) bool {
		a, b := log.IncomingSources[i], log.IncomingSources[j]
		return a.Second < b.Second || a.Second == b.Second && a.Source < b.Source
	})
}
