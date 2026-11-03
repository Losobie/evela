package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const skyhookTargetExample = "Orbital Skyhook (R3-K7K IX) [Unfinished Symphony][MOVMT](Orbital Skyhook)"
const skyhookDeflectionExample = "60 to " + skyhookTargetExample + " - Vespa II - Glances Off - Glances Off (60 deflected)"

func TestDamageDeflectionAndOptionalTargetMetadata(t *testing.T) {
	for _, c := range []struct {
		message            string
		amount, deflection float64
		entity             combatEntity
		weapon, strength   string
		hasDeflection      bool
	}{
		{"59 to " + skyhookTargetExample + " - Vespa II - Glances Off", 59, 0, combatEntity{"Orbital Skyhook", "Orbital Skyhook", "R3-K7K IX", "Unfinished Symphony", "MOVMT"}, "Vespa II", "Glances Off", false},
		{skyhookDeflectionExample, 60, 60, combatEntity{"Orbital Skyhook", "Orbital Skyhook", "R3-K7K IX", "Unfinished Symphony", "MOVMT"}, "Vespa II", "Glances Off", true},
		{"908 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates", 908, 0, combatEntity{"Grijawlva", "Phoenix Navy Issue", "", "", "SNOOO"}, "425mm Railgun II", "Penetrates", false},
		{"80 to Pilot - Name[C](Hulk) - Ogre I - Hits (12 deflected)", 80, 12, combatEntity{"Pilot - Name", "Hulk", "", "", "C"}, "Ogre I", "Hits", true},
		{"80 to Pilot[C](Hulk) - Ogre I - Penetrates - Glances Off (12 deflected)", 80, 12, combatEntity{"Pilot", "Hulk", "", "", "C"}, "Ogre I", "Penetrates", true},
		{"80 to Enemy - Ogre I - Hits - Hits (1,234.5 deflected)", 80, 1234.5, combatEntity{Name: "Enemy"}, "Ogre I", "Hits", true},
		{"80 to Enemy - Ogre I - Hits - Hits (0 deflected)", 80, 0, combatEntity{Name: "Enemy"}, "Ogre I", "Hits", true},
		{"80 to Enemy - Ogre I - Hits", 80, 0, combatEntity{Name: "Enemy"}, "Ogre I", "Hits", false},
	} {
		t.Run(c.message, func(t *testing.T) {
			log, err := parseLog(strings.NewReader("Listener: Pilot A\n[ 2026.10.19 02:00:00 ] (combat) " + c.message))
			if err != nil || len(log.Hits) != 1 {
				t.Fatalf("parse: %v, %+v", err, log)
			}
			hit := log.Hits[0]
			entity := combatEntity{hit.Target, hit.TargetType, hit.TargetLocation, hit.TargetCorporation, hit.TargetCorporationTicker}
			if hit.Amount != c.amount || hit.DeflectionAmount != c.deflection || hit.HasDeflection != c.hasDeflection || entity != c.entity || hit.WeaponType != c.weapon || hit.WeaponStrength != c.strength {
				t.Fatalf("incorrect parsed hit: %+v", hit)
			}
			if len(log.Points) != 1 || log.Points[0].Outgoing != c.amount || log.Points[0].OutgoingHits != 1 {
				t.Fatal("deflection changed the logged damage or hit count")
			}
		})
	}
}

func TestCombatEntityMissingFields(t *testing.T) {
	for _, c := range []struct {
		input string
		want  combatEntity
	}{
		{"Base (System) [Corp][TICK](Fortizar)", combatEntity{"Base", "Fortizar", "System", "Corp", "TICK"}},
		{"Base [Corp][TICK](Fortizar)", combatEntity{"Base", "Fortizar", "", "Corp", "TICK"}},
		{"Base (System) [Corp][TICK]", combatEntity{"Base", "", "System", "Corp", "TICK"}},
		{"Pilot[TICK]", combatEntity{Name: "Pilot", CorporationTicker: "TICK"}},
		{"Base(Fortizar)", combatEntity{Name: "Base", Type: "Fortizar"}},
		{"Enemy - Name", combatEntity{Name: "Enemy - Name"}},
	} {
		if got := parseCombatEntity(c.input); got != c.want {
			t.Fatalf("entity %q: %+v, want %+v", c.input, got, c.want)
		}
	}
	event, ok := parseLogistics("120 remote shield boosted to Base (System) [Corp][TICK](Fortizar) - Remote Shield Booster II", 1)
	if !ok || event.Target != "Base" || event.TargetType != "Fortizar" {
		t.Fatal("target metadata parsing regressed remote-assistance module suffixes")
	}
}

func TestIncomingDeflectionSourcesAndStructureMarkers(t *testing.T) {
	input := "Listener: Pilot A\n" +
		"[ 2026.10.19 02:00:00 ] (combat) 59 from Amarr Sentry Gun - Glances Off\n" +
		"[ 2026.10.19 02:00:00 ] (combat) 60 from Amarr Sentry Gun - Glances Off - Glances Off (60 deflected)\n" +
		"[ 2026.10.19 02:00:01 ] (combat) 10 from Pilot (System) [Corp][TICK](Hulk) - Ogre I - Hits - Hits (5 deflected)\n" +
		"[ 2026.10.19 02:00:02 ] (combat) " + skyhookDeflectionExample + "\n" +
		"[ 2026.10.19 02:00:03 ] (notify) Target is invulnerable.\n"
	log, err := parseLog(strings.NewReader(input))
	if err != nil || len(log.Hits) != 4 {
		t.Fatalf("parse: %v, %+v", err, log)
	}
	if len(log.Sources) != 2 || log.Hits[0].Source != log.Hits[1].Source || !log.Sources[0].NPC || log.Sources[0].Name != "Amarr Sentry Gun" {
		t.Fatal("deflection split or lost NPC source")
	}
	source := log.Sources[1]
	if source.Character != "Pilot" || source.Hull != "Hulk" || source.Location != "System" || source.Corporation != "Corp" || source.CorporationTicker != "TICK" {
		t.Fatalf("incoming metadata: %+v", source)
	}
	if log.Hits[1].WeaponStrength != "Glances Off" || log.Hits[1].WeaponType != "" || log.Hits[1].DeflectionAmount != 60 || log.IncomingSources[0].Damage != 119 {
		t.Fatal("incoming damage or strength changed")
	}
	if len(log.StructureEvents) != 1 || log.StructureEvents[0].Type != "Orbital Skyhook" {
		t.Fatal("deflection prevented structure invulnerability detection")
	}
}

func TestDeflectionGroupingFiltersAndSQLiteCache(t *testing.T) {
	ui, start := newDamageFilterTestUI(t)
	var input strings.Builder
	input.WriteString("Listener: Pilot A\n")
	for _, message := range []string{"59 to " + skyhookTargetExample + " - Vespa II - Glances Off", skyhookDeflectionExample, "908 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates"} {
		fmt.Fprintf(&input, "[ 2026.10.19 02:00:00 ] (combat) %s\n", message)
	}
	folder := t.TempDir()
	writeCacheLog(t, folder, "20261019_deflection.txt", input.String())
	dbPath := filepath.Join(t.TempDir(), "logs.sqlite")
	cache := testLogCache(t, dbPath, logParserVersion)
	first := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	cache.close()
	cache = testLogCache(t, dbPath, logParserVersion)
	second := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if second.CachedFiles != 1 || !reflect.DeepEqual(first.Logs, second.Logs) {
		t.Fatal("SQLite restart lost metadata or deflection fields")
	}
	ui.snapshot = second
	ui.applyGraphRange(start, start.Add(2*time.Second))
	chooseGraphMenuItem(t, ui.graphFormatMenu(), filterTarget)
	if len(ui.tableSeries) != 2 || filteredTotal(ui) != 1027 {
		t.Fatalf("target grouping split metadata or changed damage: %+v", ui.tableSeries)
	}
	ui.setAllScopeFilters(graphOutgoing, filterWeaponType, false)
	ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Vespa II", true)
	ui.setAllScopeFilters(graphOutgoing, filterWeaponStrength, false)
	ui.setScopeFilter(graphOutgoing, filterWeaponStrength, "name:Glances Off", true)
	ui.setAllScopeFilters(graphOutgoing, filterTarget, false)
	ui.setScopeFilter(graphOutgoing, filterTarget, "name:Orbital Skyhook", true)
	if len(ui.tableSeries) != 1 || filteredTotal(ui) != 119 {
		t.Fatal("deflected hit did not match the same target/weapon/strength filters")
	}
}
