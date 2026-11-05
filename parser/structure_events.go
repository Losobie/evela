package parser

import (
	"regexp"
	"strings"
	"time"
)

const structureInvulnerabilityWindow = 30 * time.Second

type StructureEvent struct {
	Second int64
	Target string
	Type   string
}

// Match the hull suffix in the outgoing target, rather than arbitrary words in
// its name. EVE target names can themselves contain " - ", as in moon drills.
var structureTargetSuffix = regexp.MustCompile(`^(.+)\(([^()]*)\)(?:\s+-\s+.*)?$`)
var structureTypeQuotes = strings.NewReplacer("‘", "'", "’", "'")

// Explicit structure hulls keep ships, NPCs and generic mission structures out.
// Upwell: https://support.eveonline.com/hc/en-us/articles/213021829
// Skyhooks/Metenox: https://www.eveonline.com/news/view/sovereignty-structures-and-transition
var playerStructureTypes = map[string]bool{
	"astrahus": true, "fortizar": true, "keepstar": true, "upwell palatine keepstar": true,
	"'draccous' fortizar": true, "'horizon' fortizar": true, "'marginis' fortizar": true,
	"'moreau' fortizar": true, "'prometheus' fortizar": true,
	"raitaru": true, "azbel": true, "sotiyo": true,
	"athanor": true, "tatara": true, "metenox moon drill": true,
	"ansiblex jump gate": true, "pharolux cyno beacon": true, "tenebrex cyno jammer": true,
	"orbital skyhook": true, "sovereignty hub": true, "infrastructure hub": true,
	"territorial claim unit": true, "sovereignty blockade unit": true,
}

var legacyPlayerStructureTypes = func() map[string]bool {
	types := map[string]bool{"customs office": true, "customs office gantry": true}
	for _, faction := range []string{"amarr", "caldari", "gallente", "minmatar", "angel", "domination", "blood", "dark blood", "guristas", "dread guristas", "sansha", "true sansha", "serpentis", "shadow"} {
		for _, size := range []string{"", " small", " medium"} {
			types[faction+" control tower"+size] = true
		}
	}
	return types
}()

func outgoingStructureTarget(target string) (string, string, bool) {
	target, _, _ = damageWithoutDeflection(strings.TrimSpace(target))
	weapon, quality := damageWeaponDetails(target)
	if damageHitQualities[quality] {
		target, _, _ = cutLastDamageDetail(target)
		if weapon != "" {
			target, _, _ = cutLastDamageDetail(target)
		}
	}
	match := structureTargetSuffix.FindStringSubmatch(strings.TrimSpace(target))
	if match == nil {
		return "", "", false
	}
	typeName := strings.TrimSpace(match[2])
	key := strings.ToLower(structureTypeQuotes.Replace(typeName))
	recognized := playerStructureTypes[key]
	if !recognized {
		if !legacyPlayerStructureTypes[key] {
			return "", "", false
		}
		// Legacy towers/customs offices can have NPC counterparts. Require the
		// player corporation ticker that precedes the type suffix in these logs.
		owner := strings.TrimSpace(match[1])
		bracket := strings.LastIndexByte(owner, '[')
		if bracket < 0 || !strings.HasSuffix(owner, "]") || len(owner)-bracket < 3 {
			return "", "", false
		}
	}
	return strings.TrimSpace(match[1]) + " (" + typeName + ")", typeName, true
}
