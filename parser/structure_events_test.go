package parser

import "testing"

func TestStructureTargetsRequireRecognizedHullSuffix(t *testing.T) {
	for _, item := range []struct {
		target string
		want   bool
	}{
		{"Valmu - Valmu 2[TLOS](Metenox Moon Drill) - Laser - Hits", true},
		{"Home[TEST](Astrahus) - Laser - Hits", true},
		{"Home[TEST](Fortizar) - Laser - Hits", true},
		{"Home[TEST](Keepstar) - Laser - Hits", true},
		{"Home[TEST](Upwell Palatine Keepstar) - Laser - Hits", true},
		{"Home[TEST](‘Draccous’ Fortizar) - Laser - Hits", true},
		{"Industry[TEST](Raitaru) - Drone - Hits", true},
		{"Industry[TEST](Azbel) - Drone - Hits", true},
		{"Industry[TEST](Sotiyo) - Drone - Hits", true},
		{"Mining[TEST](Athanor) - Laser - Hits", true},
		{"Mining[TEST](Tatara) - Laser - Hits", true},
		{"Gate[TEST](Ansiblex Jump Gate) - Laser - Hits", true},
		{"Cyno[TEST](Pharolux Cyno Beacon) - Laser - Hits", true},
		{"Jammer[TEST](Tenebrex Cyno Jammer) - Laser - Hits", true},
		{"Planet[TEST](Orbital Skyhook) - Laser - Hits", true},
		{"Sov[TEST](Sovereignty Hub) - Laser - Hits", true},
		{"Moon[TEST](Caldari Control Tower Medium) - Laser - Hits", true},
		{"Moon[TEST](True Sansha Control Tower) - Laser - Hits", true},
		{"Planet[TEST](Customs Office) - Laser - Hits", true},
		{"Astrahus[TEST](Venture) - Laser - Hits", false},
		{"Metenox Moon Drill[TEST](Loki) - Laser - Hits", false},
		{"NPC(Guristas Forward Operating Base) - Laser - Hits", false},
		{"NPC(Guristas Control Tower) - Laser - Hits", false},
		{"Planet(Customs Office) - Laser - Hits", false},
		{"NPC(Guristas Battleship) - Laser - Hits", false},
		{"Astrahus - Laser - Hits", false},
		{"Unknown Structure[TEST](Unknown Hull) - Laser - Hits", false},
	} {
		_, _, got := outgoingStructureTarget(item.target)
		if got != item.want {
			t.Errorf("target %q classified as structure=%v, want %v", item.target, got, item.want)
		}
	}
}
