package main

import (
	"sort"
	"strings"
)

const (
	filterWeaponType     = "Weapon Type"
	filterWeaponStrength = "Weapon Strength"
)

func damageWeaponDetails(detail string) (string, string) {
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
	if i := strings.LastIndex(body, " - "); i >= 0 {
		weapon = strings.TrimSpace(body[i+3:])
		body = strings.TrimSpace(body[:i])
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

func weaponGroupingMode(mode string) bool {
	return mode == filterWeaponType || mode == filterWeaponStrength
}
func weaponOption(value, kind string) damageFilterOption {
	if value == "" {
		return damageFilterOption{Key: "unknown", Label: "Unknown " + strings.ToLower(kind)}
	}
	return damageFilterOption{Key: "name:" + value, Label: value}
}
func hitWeaponOption(hit damageHit, mode string) damageFilterOption {
	if mode == filterWeaponStrength {
		return weaponOption(hit.WeaponStrength, mode)
	}
	return weaponOption(hit.WeaponType, mode)
}
func (ui *analyzerUI) weaponGrouping() bool {
	return ui.groupSelect != nil && weaponGroupingMode(ui.groupSelect.Selected)
}

func rebuildDamageFromHits(log *parsedLog) {
	points := map[int64]*damagePoint{}
	sources := map[[2]int64]*incomingSourcePoint{}
	log.Points, log.IncomingSources = nil, nil
	for _, hit := range log.Hits {
		if hit.WeaponStrength == "Misses" {
			continue
		}
		point := points[hit.Second]
		if point == nil {
			point = &damagePoint{Second: hit.Second}
			points[hit.Second] = point
		}
		if hit.Incoming {
			point.Incoming += hit.Amount
			point.IncomingHits++
			if hit.Source >= 0 {
				key := [2]int64{hit.Second, int64(hit.Source)}
				source := sources[key]
				if source == nil {
					source = &incomingSourcePoint{Second: hit.Second, Source: hit.Source}
					sources[key] = source
				}
				source.Damage += hit.Amount
				source.Hits++
			}
		} else {
			point.Outgoing += hit.Amount
			point.OutgoingHits++
		}
	}
	for _, point := range points {
		log.Points = append(log.Points, *point)
	}
	for _, point := range sources {
		log.IncomingSources = append(log.IncomingSources, *point)
	}
	sortDamageRecords(log)
}

func sortDamageRecords(log *parsedLog) {
	sort.Slice(log.Points, func(i, j int) bool { return log.Points[i].Second < log.Points[j].Second })
	sort.Slice(log.IncomingSources, func(i, j int) bool {
		a, b := log.IncomingSources[i], log.IncomingSources[j]
		return a.Second < b.Second || a.Second == b.Second && a.Source < b.Source
	})
}
