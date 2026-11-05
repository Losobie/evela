package main

import (
	"sort"
	"strconv"
	"strings"
)

type engagementIdentity struct{ name, class, group string }

type engagementIdentities struct {
	classes map[string]string
	targets map[[2]string]engagementIdentity
	sources map[damageSource]engagementIdentity
}

func newEngagementIdentities(ships shipHistory) *engagementIdentities {
	classes := map[string]string{}
	for name, class := range loadHullClasses() {
		classes[name] = class
	}
	for _, periods := range ships {
		for _, ship := range periods {
			key := characterKey(ship.TypeName)
			if classes[key] == "" && ship.ClassName != "" {
				classes[key] = ship.ClassName
			}
		}
	}
	return &engagementIdentities{classes: classes, targets: map[[2]string]engagementIdentity{}, sources: map[damageSource]engagementIdentity{}}
}

func (identities *engagementIdentities) hull(name string) engagementIdentity {
	if name == "" {
		return engagementIdentity{}
	}
	class := identities.classes[characterKey(name)]
	group := "hull:" + class
	if class == "" {
		group = "hull-type:" + name
	}
	return engagementIdentity{name, class, group}
}

func engagementNPC(name string, groupID int) engagementIdentity {
	group, class := "npc-name:"+name, ""
	if groupID > 0 {
		class = loadNPCCatalog().Groups[groupID]
		if class != "" {
			group = "npc-group:" + strconv.Itoa(groupID)
		}
	}
	return engagementIdentity{name, class, group}
}

func (identities *engagementIdentities) target(name, hull string) engagementIdentity {
	key := [2]string{name, hull}
	if identity, ok := identities.targets[key]; ok {
		return identity
	}
	identity := identities.hull(hull)
	if class := identities.classes[characterKey(name)]; class != "" && (hull == "" || hull == class) {
		// A catalog fallback stores the class in Target Type. Keep the specific
		// SDE name available for single-type engagement summaries.
		identity = identities.hull(name)
	} else if hull == "" {
		if npc, ok := loadNPCCatalog().names[characterKey(name)]; ok {
			identity = engagementNPC(name, npc.GroupID)
		}
	}
	identities.targets[key] = identity
	return identity
}

func (identities *engagementIdentities) source(source damageSource) engagementIdentity {
	if identity, ok := identities.sources[source]; ok {
		return identity
	}
	identity := identities.hull(source.Hull)
	if source.NPC {
		identity = engagementNPC(source.Name, source.GroupID)
	}
	identities.sources[source] = identity
	return identity
}

type engagementTypeGroup struct {
	class  string
	types  map[string]bool
	weight int
}

type engagementTypes map[string]*engagementTypeGroup

func (types engagementTypes) add(identity engagementIdentity) {
	if identity.name == "" {
		return
	}
	group := types[identity.group]
	if group == nil {
		group = &engagementTypeGroup{class: identity.class, types: map[string]bool{}}
		types[identity.group] = group
	}
	group.types[identity.name] = true
	group.weight++
}

func engagementPlural(name string) string {
	if strings.HasSuffix(name, "s") {
		return name
	}
	if strings.HasSuffix(name, "y") {
		return strings.TrimSuffix(name, "y") + "ies"
	}
	return name + "s"
}

func (types engagementTypes) names() string {
	weights := map[string]int{}
	for _, group := range types {
		name := engagementPlural(group.class)
		if len(group.types) == 1 || group.class == "" {
			for hull := range group.types {
				name = hull
			}
		}
		weights[name] += group.weight
	}
	return strings.Join(engagementTopNames(weights, 2), " and ")
}

func engagementTopNames(names map[string]int, limit int) []string {
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Slice(keys, func(i, j int) bool {
		if names[keys[i]] != names[keys[j]] {
			return names[keys[i]] > names[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys[:min(len(keys), limit)]
}

type engagementSeenKey struct {
	activity    int
	name, group string
}

type engagementSummary struct {
	first, last            int64
	activitySeconds        [4]int
	directions             [6]int
	ships, opponents       [6]engagementTypes
	systems, ores, effects map[string]int
	seen                   map[engagementSeenKey]int64
	mined                  bool
}

func newEngagementSummary(second int64) *engagementSummary {
	summary := &engagementSummary{first: second, last: second, systems: map[string]int{}, ores: map[string]int{}, effects: map[string]int{}, seen: map[engagementSeenKey]int64{}}
	for i := range summary.ships {
		summary.ships[i], summary.opponents[i] = engagementTypes{}, engagementTypes{}
	}
	return summary
}

func (summary *engagementSummary) firstAt(activity int, name string, second int64) bool {
	return summary.firstAtKey(engagementSeenKey{activity: activity, name: name}, second)
}

func (summary *engagementSummary) firstAtKey(key engagementSeenKey, second int64) bool {
	if previous, ok := summary.seen[key]; ok && previous == second {
		return false
	}
	summary.seen[key] = second
	return true
}

func (summary *engagementSummary) add(cursor engagementCursor, ship engagementIdentity, identities *engagementIdentities) {
	summary.last = cursor.second
	activity := 0
	var opponent engagementIdentity
	switch cursor.kind {
	case engagementDamage:
		hit := cursor.log.Hits[cursor.index]
		if hit.Incoming {
			activity = 1
			if hit.Source >= 0 && hit.Source < len(cursor.log.Sources) {
				opponent = identities.source(cursor.log.Sources[hit.Source])
			}
		} else {
			opponent = identities.target(hit.Target, hit.TargetType)
		}
	case engagementEWar:
		activity = 2
		effect := cursor.log.EWar[cursor.index].Effect
		if effect != "" && summary.firstAt(20, effect, cursor.second) {
			summary.effects[effect]++
		}
	case engagementLogistics:
		activity = 3
		event := cursor.log.Logistics[cursor.index]
		if event.Incoming {
			activity = 4
		} else {
			opponent = identities.target(event.Target, event.TargetType)
		}
	case engagementMining:
		activity = 5
		event := cursor.log.Mining[cursor.index]
		summary.mined = summary.mined || event.Kind != miningCompressed
		if event.Ore != "" && summary.firstAt(21, event.Ore, cursor.second) {
			summary.ores[event.Ore]++
		}
	}
	// Count active seconds per character, rather than weapon / drone message
	// frequency or incomparable damage / ore amounts. Combat gets triple weight.
	if summary.firstAt(10+cursor.kind, cursor.log.Character, cursor.second) {
		summary.activitySeconds[cursor.kind]++
	}
	if summary.firstAt(activity, cursor.log.Character, cursor.second) {
		summary.directions[activity]++
		summary.ships[activity].add(ship)
	}
	if opponent.name != "" && summary.firstAtKey(engagementSeenKey{activity: 30 + activity, name: opponent.name, group: opponent.group}, cursor.second) {
		summary.opponents[activity].add(opponent)
	}
}

func (summary *engagementSummary) text() string {
	dominant, best := engagementDamage, summary.activitySeconds[engagementDamage]*3
	if mining := summary.activitySeconds[engagementMining]; mining > best {
		dominant, best = engagementMining, mining
	}
	// Combat and mining identify the engagement. Repair / E-War message volume
	// must not drown them out; use support activity only when neither is present.
	if best == 0 {
		for _, kind := range []int{engagementLogistics, engagementEWar} {
			if score := summary.activitySeconds[kind]; score > best {
				dominant, best = kind, score
			}
		}
	}
	activity, action := 0, "Combat"
	switch dominant {
	case engagementDamage:
		if summary.directions[1] > summary.directions[0] {
			activity, action = 1, "Taking fire"
			if opponents := summary.opponents[1].names(); opponents != "" {
				action += " from " + opponents
			}
		} else if opponents := summary.opponents[0].names(); opponents != "" {
			action = "Shooting " + opponents
		}
	case engagementMining:
		activity, action = 5, "Mining"
		if !summary.mined {
			action = "Compressing"
		}
		if ores := engagementTopNames(summary.ores, 2); len(ores) > 0 {
			action += " " + strings.Join(ores, " and ")
		}
	case engagementLogistics:
		activity, action = 3, "Providing logistics"
		if summary.directions[4] > summary.directions[3] {
			activity, action = 4, "Receiving logistics"
		} else if targets := summary.opponents[3].names(); targets != "" {
			action = "Supporting " + targets
		}
	case engagementEWar:
		activity, action = 2, "Under E-War"
		if effects := engagementTopNames(summary.effects, 2); len(effects) > 0 {
			action += " (" + strings.Join(effects, " and ") + ")"
		}
	}
	if ships := summary.ships[activity].names(); ships != "" {
		action += " with " + ships
	}
	systems := strings.Join(engagementTopNames(summary.systems, 3), ", ")
	if systems == "" {
		systems = "Unknown system"
	}
	return systems + " - " + action
}
