package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestEngagementSummaryExamples(t *testing.T) {
	start := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, target, ore string
		hulls, systems    []string
		want              string
	}{
		{"ships", "Drake Navy Issue", "", []string{"Kikimora"}, []string{"R3-K7K"}, "R3-K7K - Shooting Drake Navy Issue with Kikimora"},
		{"mixed hulls", "Metenox Moon Drill", "", []string{"Nyx", "Hel", "Kikimora"}, []string{"Misaba", "Misaba", "Mamet"}, "Misaba, Mamet - Shooting Metenox Moon Drill with Supercarriers and Kikimora"},
		{"one mining hull", "", "Xenotime", []string{"Hulk", "Hulk"}, []string{"Sieh", "Sieh"}, "Sieh - Mining Xenotime with Hulk"},
		{"mixed mining hulls", "", "Xenotime", []string{"Hulk", "Mackinaw"}, []string{"Sieh", "Sieh"}, "Sieh - Mining Xenotime with Exhumers"},
		{"unknown mining hull", "", "Xenotime", []string{""}, []string{"Sieh"}, "Sieh - Mining Xenotime"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := &logSnapshot{}
			ships, locations := shipHistory{}, locationHistory{}
			for i, hull := range tc.hulls {
				character := fmt.Sprintf("Pilot %d", i)
				log := &parsedLog{Character: character}
				if tc.ore != "" {
					log.Mining = []miningEvent{{Second: start.Unix(), Ore: tc.ore, Kind: miningMined}}
				} else {
					log.Hits = []damageHit{{Second: start.Unix(), Target: "Named target", TargetType: tc.target}}
				}
				snapshot.Logs = append(snapshot.Logs, log)
				id := int64(i + 1)
				ships[id] = []shipPeriod{{CharacterID: id, CharacterName: character, TypeName: hull, FirstSeen: start, LastSeen: start}}
				locations[id] = []locationPeriod{{CharacterID: id, CharacterName: character, SystemName: tc.systems[i], FirstSeen: start, LastSeen: start}}
			}
			rows, err := buildEngagements(context.Background(), snapshot, newLocationResolver(locations, nil), ships)
			if err != nil || len(rows) != 1 || rows[0].Summary != tc.want {
				t.Fatalf("got %+v (%v), want %q", rows, err, tc.want)
			}
			slices.Reverse(snapshot.Logs)
			again, err := buildEngagements(context.Background(), snapshot, newLocationResolver(locations, nil), ships)
			if err != nil || len(again) != 1 || again[0] != rows[0] {
				t.Fatalf("equal-timestamp log ordering changed the summary: %+v, %v", again, err)
			}
		})
	}
}

func TestEngagementSummaryOtherActivities(t *testing.T) {
	for _, tc := range []struct {
		name string
		log  parsedLog
		want string
	}{
		{"logistics out", parsedLog{Logistics: []logisticsEvent{{Second: 1, Amount: 100, Target: "Friend", TargetType: "Basilisk", Kind: "Shield repair"}}}, "Unknown system - Supporting Basilisk"},
		{"logistics in", parsedLog{Logistics: []logisticsEvent{{Second: 1, Amount: 100, Incoming: true, Kind: "Shield repair"}}}, "Unknown system - Receiving logistics"},
		{"E-War", parsedLog{EWar: []ewarEvent{{Second: 1, Effect: "Warp scramble"}}}, "Unknown system - Under E-War (Warp scramble)"},
		{"compression", parsedLog{Mining: []miningEvent{{Second: 1, Ore: "Xenotime", Kind: miningCompressed}}}, "Unknown system - Compressing Xenotime"},
		{"unknown target", parsedLog{Hits: []damageHit{{Second: 1, Target: "Unresolved name"}}}, "Unknown system - Combat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := buildEngagements(context.Background(), &logSnapshot{Logs: []*parsedLog{&tc.log}}, locationResolver{}, nil)
			if err != nil || len(rows) != 1 || rows[0].Summary != tc.want {
				t.Fatalf("got %+v (%v), want %q", rows, err, tc.want)
			}
		})
	}
}

func TestEngagementSummaryCombatBiasAndMessageFrequency(t *testing.T) {
	for _, combatSeconds := range []int{1, 2} {
		snapshot := &logSnapshot{Logs: []*parsedLog{{Character: "Pilot"}}}
		log := snapshot.Logs[0]
		for second := 0; second < combatSeconds; second++ {
			for hit := 0; hit < 1000; hit++ {
				log.Hits = append(log.Hits, damageHit{Second: int64(second), TargetType: "Drake Navy Issue"})
			}
		}
		for second := 0; second < 4; second++ {
			log.Mining = append(log.Mining, miningEvent{Second: int64(second), Ore: "Xenotime", Kind: miningMined}, miningEvent{Second: int64(second), Ore: "Xenotime", Kind: miningWaste})
		}
		rows, err := buildEngagements(context.Background(), snapshot, locationResolver{}, nil)
		want := "Unknown system - Mining Xenotime"
		if combatSeconds == 2 {
			want = "Unknown system - Shooting Drake Navy Issue"
		}
		if err != nil || len(rows) != 1 || rows[0].Summary != want {
			t.Fatalf("%d combat seconds: %+v (%v), want %s", combatSeconds, rows, err, want)
		}
	}
}

func TestEngagementSummaryTypesAndSystemLimits(t *testing.T) {
	identities := newEngagementIdentities(nil)
	summary := newEngagementSummary(0)
	for name, weight := range map[string]int{"Misaba": 4, "Mamet": 3, "Sieh": 2, "Amarr": 1} {
		summary.systems[name] = weight
	}
	summary.activitySeconds[engagementDamage] = 5
	for _, hull := range []string{"Caracal", "Caracal", "Caracal", "Vedmak", "Raven", "Raven", "Rifter"} {
		summary.opponents[0].add(identities.hull(hull))
	}
	for _, hull := range []string{"Nyx", "Nyx", "Hel", "Kikimora", "Kikimora", "Hulk"} {
		summary.ships[0].add(identities.hull(hull))
	}
	want := "Misaba, Mamet, Sieh - Shooting Cruisers and Raven with Supercarriers and Kikimora"
	if got := summary.text(); got != want {
		t.Fatalf("grouping / limits = %q, want %q", got, want)
	}
	// One known NPC name stays specific; several names in one NPC type collapse.
	summary = newEngagementSummary(0)
	summary.activitySeconds[engagementDamage], summary.directions[1] = 2, 2
	summary.opponents[1].add(identities.source(damageSource{Name: "Amarr Sentry Gun", NPC: true, GroupID: 99}))
	if got := summary.text(); got != "Unknown system - Taking fire from Amarr Sentry Gun" {
		t.Fatalf("single NPC = %s", got)
	}
	summary.opponents[1].add(identities.source(damageSource{Name: "Gallente Sentry Gun", NPC: true, GroupID: 99}))
	if got := summary.text(); got != "Unknown system - Taking fire from Sentry Guns" {
		t.Fatalf("NPC group = %s", got)
	}
	if got := identities.target("Amarr Sentry Gun", ""); got.name != "Amarr Sentry Gun" || got.class != "Sentry Gun" {
		t.Fatalf("outgoing NPC identification = %+v", got)
	}
}

func TestEngagementSummaryUsesShipAtActivityTime(t *testing.T) {
	start := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	snapshot := &logSnapshot{Logs: []*parsedLog{{Character: "Pilot", Mining: []miningEvent{{Second: start.Unix(), Ore: "Xenotime", Kind: miningMined}}}}}
	ships := shipHistory{1: {
		{CharacterID: 1, CharacterName: "Pilot", TypeName: "Hulk", FirstSeen: start, LastSeen: start},
		{CharacterID: 1, CharacterName: "Pilot", TypeName: "Kikimora", FirstSeen: start.Add(time.Hour), LastSeen: start.Add(time.Hour)},
	}}
	rows, err := buildEngagements(context.Background(), snapshot, locationResolver{}, ships)
	if err != nil || len(rows) != 1 || rows[0].Summary != "Unknown system - Mining Xenotime with Hulk" {
		t.Fatalf("new ship was assigned to old activity: %+v, %v", rows, err)
	}
}

func TestEngagementTableFitsViewport(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Responsive engagement list")
	t.Cleanup(window.Close)
	viewer := newEngagementsViewer(func(engagement) {})
	for i := 0; i < 200; i++ {
		viewer.rows = append(viewer.rows, engagement{Start: time.Unix(int64(i), 0), End: time.Unix(int64(i+60), 0), Summary: strings.Repeat("Long summary ", 100)})
	}
	viewer.initTable()
	holder := newFillingTable(&viewer.table.Table, 220, 100)
	window.SetContent(holder)
	window.Show()
	var scroll *container.Scroll
	for _, object := range test.WidgetRenderer(viewer.table).Objects() {
		if candidate, ok := object.(*container.Scroll); ok {
			scroll = candidate
		}
	}
	if scroll == nil {
		t.Fatal("table scroll viewport missing")
	}
	for _, width := range []float32{480, 1440, 900, 520} {
		holder.Resize(fyne.NewSize(width, 400))
		viewer.table.Select(widget.TableCellID{Row: len(viewer.rows), Col: 2})
		contentWidth := scroll.Content.MinSize().Width
		if contentWidth > scroll.Size().Width+0.1 || scroll.Offset.X > 0.1 || contentWidth < scroll.Size().Width-0.1 {
			t.Fatalf("width %.0f: table content %.1f / viewport %.1f, horizontal offset %.1f", width, contentWidth, scroll.Size().Width, scroll.Offset.X)
		}
		if scroll.Offset.Y <= 0 {
			t.Fatal("fitting the table width disabled vertical scrolling")
		}
	}
}
