package parser

import (
	"encoding/json"
	"testing"
)

func TestNPCSourceResolution(t *testing.T) {
	for _, tc := range []struct {
		text            string
		name            string
		npc             bool
		typeID, groupID int
	}{
		{"Guristas Blackbird - Guristas Inferno Heavy Missile - Hits", "Guristas Blackbird", true, 76045, 1568},
		{"Amarr Sentry Gun - Glances Off", "Amarr Sentry Gun", true, 1194, 99},
		{"Sansha's Lord - Mjolnir Cruise Missile - Hits", "Sansha's Lord", true, 11913, 565},
		{"  amarr sentry gun - Hits", "amarr sentry gun", true, 1194, 99},
		{"アマーセントリーガン - Hits", "アマーセントリーガン", true, 1194, 99},
		{"Guristas Blackbird[CORP](Raven) - Missile (Faction) - Hits", "Guristas Blackbird[CORP](Raven)", false, 0, 0},
		{"Moon - Mining[CORP](Tatara) - Standup Heavy Missile - Hits", "Moon - Mining[CORP](Tatara)", false, 0, 0},
		{"Brand new unknown rat - Laser - Hits", "Brand new unknown rat", false, 0, 0},
		{"Amarr Sentry Gun - Unknown variant - Laser - Hits", "Amarr Sentry Gun - Unknown variant", false, 0, 0},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got := IncomingDamageSource(tc.text)
			if got.Name != tc.name || got.NPC != tc.npc || got.TypeID != tc.typeID || got.GroupID != tc.groupID {
				t.Fatalf("source resolution: %+v", got)
			}
		})
	}
	if catalog := loadNPCCatalog(); catalog.Build == 0 || len(catalog.names) < 7000 || catalog.Groups[99] != "Sentry Gun" {
		t.Fatal("embedded official NPC lookup is incomplete")
	}
}

func TestNPCDuplicateNamesRetainOnlyUnambiguousIDs(t *testing.T) {
	catalog := &npcCatalog{}
	err := json.Unmarshal([]byte(`{"types":[{"id":1,"group":10,"names":["Shared group"]},{"id":2,"group":10,"names":["Shared group"]},{"id":3,"group":11,"names":["Ambiguous"]},{"id":4,"group":12,"names":["Ambiguous"]},{"id":5,"group":12,"names":["Ambiguous"]}]}`), catalog)
	if err != nil {
		t.Fatal(err)
	}
	catalog.index()
	if got := catalog.names["shared group"]; got.TypeID != 0 || got.GroupID != 10 {
		t.Fatalf("shared group: %+v", got)
	}
	if got := catalog.names["ambiguous"]; got.TypeID != 0 || got.GroupID != 0 {
		t.Fatalf("ambiguous type: %+v", got)
	}
}
