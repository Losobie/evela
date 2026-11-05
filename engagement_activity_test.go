package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEngagementsIgnoreEmptyRepairsAndCapChains(t *testing.T) {
	input := "Listener: Logistics\n" +
		"[ 2026.11.04 12:00:00 ] (combat) 0 remote shield boosted to Stork [53C0N] [Alpha Arterius] - - Large Remote Shield Booster II\n" +
		"[ 2026.11.04 12:01:00 ] (combat) 0 remote armor repaired by Friend[C](Guardian)\n" +
		"[ 2026.11.04 12:02:00 ] (combat) 299 remote capacitor transmitted to Basilisk [THIRD] [.AE..] [Clento Loso] - - Large Inductive Compact Remote Capacitor Transmitter\n" +
		"[ 2026.11.04 12:03:00 ] (combat) 299 remote capacitor transmitted by Friend[C](Guardian)\n"
	log, err := parseLog(strings.NewReader(input))
	if err != nil || len(log.Logistics) != 4 || len(log.Events) != 4 {
		t.Fatalf("fixture did not retain logistics / raw events: %+v, %v", log, err)
	}
	rows, err := buildEngagements(context.Background(), &logSnapshot{Logs: []*parsedLog{log}}, locationResolver{}, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("background logistics created engagements: %+v, %v", rows, err)
	}
	if len(log.Logistics) != 4 || len(log.Events) != 4 || log.Logistics[2].Amount != 299 {
		t.Fatal("engagement filtering changed the source records used by graphs / Events")
	}
}

func TestEngagementsBackgroundLogisticsDoNotBridgeOrExtendCombat(t *testing.T) {
	start := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	combat := &parsedLog{Character: "Combat", Hits: []damageHit{
		{Second: start.Unix(), Incoming: true, Source: -1, WeaponStrength: "Misses"},
		{Second: start.Add(20 * time.Minute).Unix(), Amount: 100, TargetType: "Raven"},
	}}
	background := &parsedLog{Character: "Background"}
	for second := -1200; second <= 2400; second += 60 {
		for _, incoming := range []bool{false, true} {
			background.Logistics = append(background.Logistics,
				logisticsEvent{Second: start.Unix() + int64(second), Amount: 200, Incoming: incoming, Kind: "Capacitor transfer", TargetType: "Basilisk"},
				logisticsEvent{Second: start.Unix() + int64(second), Amount: 0, Incoming: incoming, Kind: "Shield repair", TargetType: "Basilisk"},
				logisticsEvent{Second: start.Unix() + int64(second), Amount: 0, Incoming: incoming, Kind: "Armor repair", TargetType: "Basilisk"})
		}
	}
	ships := shipHistory{
		1: {{CharacterID: 1, CharacterName: "Combat", TypeName: "Kikimora", FirstSeen: start, LastSeen: start.Add(20 * time.Minute)}},
		2: {{CharacterID: 2, CharacterName: "Background", TypeName: "Basilisk", FirstSeen: start.Add(-20 * time.Minute), LastSeen: start.Add(40 * time.Minute)}},
	}
	locations := newLocationResolver(locationHistory{
		1: {{CharacterID: 1, CharacterName: "Combat", SystemName: "Mamet", FirstSeen: start, LastSeen: start.Add(20 * time.Minute)}},
		2: {{CharacterID: 2, CharacterName: "Background", SystemName: "Jita", FirstSeen: start.Add(-20 * time.Minute), LastSeen: start.Add(40 * time.Minute)}},
	}, nil)
	rows, err := buildEngagements(context.Background(), &logSnapshot{Logs: []*parsedLog{combat, background}}, locations, ships)
	if err != nil || len(rows) != 2 {
		t.Fatalf("idle cap chain merged separate fights: %+v, %v", rows, err)
	}
	for i, second := range []int64{1200, 0} {
		if !rows[i].Start.Equal(start.Add(time.Duration(second)*time.Second)) || rows[i].End.Sub(rows[i].Start) != time.Second {
			t.Fatalf("background activity extended engagement %d: %+v", i, rows[i])
		}
	}
	if rows[0].Summary != "Mamet - Shooting Raven with Kikimora" || rows[1].Summary != "Mamet - Taking fire with Kikimora" {
		t.Fatalf("background systems / hulls contaminated combat or zero-damage combat was lost: %+v", rows)
	}
}

func TestEngagementSummariesPreferCombatAndSustainedMiningOverSupport(t *testing.T) {
	for _, tc := range []struct {
		name          string
		miningSeconds int
		incoming      bool
		want          string
	}{
		{"outgoing combat", 0, false, "Unknown system - Shooting Raven"},
		{"incoming combat", 0, true, "Unknown system - Taking fire from Raven"},
		{"sustained mining with occasional combat", 20, true, "Unknown system - Mining Xenotime"},
		{"occasional mining during combat", 2, false, "Unknown system - Shooting Raven"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			main := &parsedLog{Character: "Main", Sources: []damageSource{{Name: "Enemy", Hull: "Raven"}}}
			for second := 1; second <= 2; second++ {
				main.Hits = append(main.Hits, damageHit{Second: int64(second), Amount: 100, Incoming: tc.incoming, Source: 0, TargetType: "Raven"})
			}
			for second := 0; second < tc.miningSeconds; second++ {
				main.Mining = append(main.Mining, miningEvent{Second: int64(second), Kind: miningMined, Ore: "Xenotime"}, miningEvent{Second: int64(second), Kind: miningWaste, Ore: "Xenotime"})
			}
			support := &parsedLog{Character: "Support"}
			for second := 0; second < 200; second++ {
				support.Logistics = append(support.Logistics, logisticsEvent{Second: int64(second), Kind: "Shield repair", Amount: 100, TargetType: "Basilisk"})
				support.EWar = append(support.EWar, ewarEvent{Second: int64(second), Effect: "Warp scramble"})
			}
			rows, err := buildEngagements(context.Background(), &logSnapshot{Logs: []*parsedLog{main, support}}, locationResolver{}, nil)
			if err != nil || len(rows) != 1 || rows[0].Summary != tc.want {
				t.Fatalf("support volume changed dominant activity: %+v, %v; want %s", rows, err, tc.want)
			}
		})
	}
}
