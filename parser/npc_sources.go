package parser

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// NPC names and groups are derived from CCP's official Static Data Export.
// EVE Online and its game data are copyright CCP hf.
// See docs/DEVELOPMENT.md for the source, version and regeneration command.
//
//go:embed npc_data.json.gz
var npcData []byte

type NPCIdentity struct{ TypeID, GroupID int }

type npcCatalog struct {
	Build   int            `json:"build"`
	Release string         `json:"release"`
	Groups  map[int]string `json:"groups"`
	Types   []struct {
		ID      int      `json:"id"`
		GroupID int      `json:"group"`
		Names   []string `json:"names"`
	} `json:"types"`
	names map[string]NPCIdentity
}

func (catalog *npcCatalog) index() {
	catalog.names = map[string]NPCIdentity{}
	for _, item := range catalog.Types {
		for _, name := range item.Names {
			key := nameKey(name)
			identity := NPCIdentity{TypeID: item.ID, GroupID: item.GroupID}
			if previous, exists := catalog.names[key]; exists {
				// Several NPC types can share a display name. Keep only IDs that
				// every candidate agrees on, rather than picking an arbitrary type.
				if previous.TypeID != identity.TypeID {
					identity.TypeID = 0
				}
				if previous.GroupID != identity.GroupID {
					identity.GroupID = 0
				}
			}
			catalog.names[key] = identity
		}
	}
}

var loadNPCCatalog = sync.OnceValue(func() *npcCatalog {
	reader, err := gzip.NewReader(bytes.NewReader(npcData))
	if err != nil {
		panic(fmt.Sprintf("embedded NPC data: %v", err))
	}
	defer reader.Close()
	catalog := &npcCatalog{}
	if err := json.NewDecoder(reader).Decode(catalog); err != nil {
		panic(fmt.Sprintf("embedded NPC data: %v", err))
	}
	catalog.index()
	catalog.Types = nil
	return catalog
})

type DamageSource struct {
	Name                                     string
	Character, Hull                          string
	NPC                                      bool
	TypeID, GroupID                          int
	Location, Corporation, CorporationTicker string
}

// Sources are interned per log; contributions retain individual attackers even
// when NPCs and players hit the same character during the same second.
type IncomingSourcePoint struct {
	Second int64
	Source int
	Damage float64
	Hits   int
}

var playerDamageSource = regexp.MustCompile(`^((.+)\[[^\[\]]+\]\(([^()]+)\))(?:\s+-\s+.*)?$`)
var damageHitQualities = map[string]bool{
	"Hits": true, "Glances Off": true, "Grazes": true,
	"Penetrates": true, "Smashes": true, "Wrecks": true,
}

func IncomingDamageSource(target string) DamageSource {
	target = strings.TrimSpace(target)
	target, _, _ = damageWithoutDeflection(target)
	// Player-owned ships and structures carry a corporation/hull suffix. Never
	// identify one as an NPC by a partial name match before that suffix.
	if suffix := playerDamageSource.FindStringSubmatch(target); suffix != nil {
		source := DamageSource{Name: strings.Clone(suffix[1]), Hull: strings.Clone(strings.TrimSpace(suffix[3]))}
		entity := ParseCombatEntity(suffix[1])
		source.Location, source.Corporation, source.CorporationTicker = entity.Location, entity.Corporation, entity.CorporationTicker
		structure := strings.ToLower(structureTypeQuotes.Replace(source.Hull))
		if !playerStructureTypes[structure] && !legacyPlayerStructureTypes[structure] {
			source.Character = entity.Name
		}
		return source
	}
	catalog := loadNPCCatalog()
	if last := strings.LastIndex(target, " - "); last >= 0 && damageHitQualities[target[last+3:]] {
		target = strings.TrimSpace(target[:last])
	}
	// Prefer the full exact SDE name, then remove at most one weapon suffix.
	// Do not keep truncating an unknown name until it resembles a known NPC.
	candidates := []string{target}
	if last := strings.LastIndex(target, " - "); last >= 0 {
		candidates = append(candidates, strings.TrimSpace(target[:last]))
	}
	for _, candidate := range candidates {
		if identity, exists := catalog.names[nameKey(candidate)]; exists {
			return DamageSource{Name: strings.Clone(candidate), NPC: true, TypeID: identity.TypeID, GroupID: identity.GroupID}
		}
	}
	return DamageSource{Name: strings.Clone(candidates[len(candidates)-1])}
}
