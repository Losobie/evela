package main

import (
	"strings"
	"testing"
)

func TestTargetTypeCatalogFallback(t *testing.T) {
	for _, tc := range []struct{ message, name, typ string }{
		{"466 to Blood Raiders Stronghold - Ogre I - Penetrates", "Blood Raiders Stronghold", "♦ Stronghold"},
		{"376 to Guristas Pirates Stronghold - Ogre I - Hits", "Guristas Pirates Stronghold", "♦ Stronghold"},
		{"466 to blood raiders stronghold - Ogre I - Penetrates", "blood raiders stronghold", "♦ Stronghold"},
		{"You miss Blood Raiders Stronghold completely - Ogre I", "Blood Raiders Stronghold", "♦ Stronghold"},
		{"466 to Blood Raiders Stronghold[C](Fortizar) - Ogre I - Hits", "Blood Raiders Stronghold", "Fortizar"},
		{"466 to Blood Raiders Stronghold Annex - Ogre I - Hits", "Blood Raiders Stronghold Annex", ""},
		{"466 to Unresolved target - Ogre I - Hits", "Unresolved target", ""},
	} {
		t.Run(tc.message, func(t *testing.T) {
			log, err := parseLog(strings.NewReader("Listener: Pilot\n[ 2026.11.04 12:00:00 ] (combat) " + tc.message))
			if err != nil || len(log.Hits) != 1 || log.Hits[0].Target != tc.name || log.Hits[0].TargetType != tc.typ {
				t.Fatalf("parsed %+v (%v), want %q / %q", log, err, tc.name, tc.typ)
			}
		})
	}
	// Remote assistance shares the fallback, including a module suffix without
	// the parenthesized hull or corporation suffix used by player targets.
	event, ok := parseLogistics("472 remote shield boosted to Blood Raiders Stronghold - Large Remote Shield Booster II", 1)
	if !ok || event.Target != "Blood Raiders Stronghold" || event.TargetType != "♦ Stronghold" {
		t.Fatalf("logistics catalog fallback: %+v", event)
	}
}

func TestTargetTypeFallbackEngagementNames(t *testing.T) {
	identities := newEngagementIdentities(nil)
	types := engagementTypes{}
	types.add(identities.target("Blood Raiders Stronghold", "♦ Stronghold"))
	if got := types.names(); got != "Blood Raiders Stronghold" {
		t.Fatalf("single structure lost its specific name: %s", got)
	}
	types.add(identities.target("Guristas Pirates Stronghold", "♦ Stronghold"))
	if got := types.names(); got != "♦ Strongholds" {
		t.Fatalf("multiple Strongholds did not combine by class: %s", got)
	}
	if got := identities.target("Blood Raiders Stronghold", "Fortizar"); got.name != "Fortizar" {
		t.Fatalf("explicit hull was replaced by catalog fallback: %+v", got)
	}
}
