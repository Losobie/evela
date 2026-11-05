package main

import (
	"fmt"
)

const (
	filterNPCNames     = "NPC Names"
	filterNPCTypes     = "NPC Types"
	filterOtherSources = "other"
)

func npcFilterOptions(source damageSource) (damageFilterOption, damageFilterOption) {
	other := damageFilterOption{Key: filterOtherSources, Label: "Other / unresolved"}
	if !source.NPC {
		return other, other
	}
	name := damageFilterOption{Key: "npc-name:" + characterKey(source.Name), Label: source.Name}
	group := damageFilterOption{Key: "unknown", Label: "Ambiguous NPC type"}
	if source.GroupID > 0 {
		group = damageFilterOption{Key: fmt.Sprintf("npc-group:%d", source.GroupID), Label: npcGroup(source.GroupID)}
	}
	return name, group
}
