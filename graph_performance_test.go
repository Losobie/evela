package main

import (
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
)

func benchmarkGraphUpdates(b *testing.B) *analyzerUI {
	b.Helper()
	application := test.NewApp()
	b.Cleanup(application.Quit)
	window := application.NewWindow("Graph benchmark")
	b.Cleanup(window.Close)
	ui := newAnalyzerUI(window)
	snapshot := &logSnapshot{}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for character := 0; character < 12; character++ {
		name := fmt.Sprintf("Pilot %02d", character)
		log := &parsedLog{Character: name, Sources: []damageSource{{Name: "Attacker[C](Hulk)", Hull: "Hulk"}}}
		for index := 0; index < 24000; index++ {
			second := start.Unix() + int64(index*10)
			weapon := "Laser"
			if index%2 == 0 {
				weapon = "Drone"
			}
			log.Hits = append(log.Hits, damageHit{Second: second, Amount: 100, Source: -1, WeaponType: weapon, WeaponStrength: "Hits"}, damageHit{Second: second, Amount: 50, Source: 0, Incoming: true, WeaponType: weapon, WeaponStrength: "Hits"})
			log.Points = append(log.Points, damagePoint{Second: second, Outgoing: 100, Incoming: 50, OutgoingHits: 1, IncomingHits: 1})
			log.IncomingSources = append(log.IncomingSources, incomingSourcePoint{Second: second, Source: 0, Damage: 50, Hits: 1})
		}
		snapshot.Logs = append(snapshot.Logs, log)
		snapshot.Characters = append(snapshot.Characters, name)
	}
	snapshot.First = start
	snapshot.Last = start.Add(239990 * time.Second)
	ui.snapshot = snapshot
	ui.visibleFilters[graphOutgoing][filterWeaponType] = true
	selection := ui.storedScopeFilter(graphOutgoing, filterWeaponType)
	selection.Default = false
	selection.set("name:Laser", true)
	ui.applyGraphRange(snapshot.Last.Add(-time.Minute), snapshot.Last.Add(time.Second))
	return ui
}

// Exercise a 61-second view of 576,000 hits across 12 characters. Archive setup
// is excluded from timing. This measures UI update processing in Fyne's test
// driver, not native window painting or end-to-end input latency.
func BenchmarkGraphUpdates(b *testing.B) {
	b.Run("ZoomWithFilter", func(b *testing.B) {
		ui := benchmarkGraphUpdates(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ui.rebuildView()
		}
	})
	b.Run("FilterChange", func(b *testing.B) {
		ui := benchmarkGraphUpdates(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Drone", i%2 == 0)
		}
	})
}

// Broad ranges with real Events records and substantial ship history reproduce
// the allocation-heavy paths identified by the interactive profile.
func BenchmarkBroadGraphUpdates(b *testing.B) {
	for _, kind := range []string{"Zoom", "Filter"} {
		b.Run(kind, func(b *testing.B) {
			ui := benchmarkGraphUpdates(b)
			start := ui.snapshot.First
			for character, log := range ui.snapshot.Logs {
				for _, point := range log.Points {
					log.Events = append(log.Events, logEvent{Second: point.Second, Message: "(combat) 100 to Enemy - Laser - Hits"})
				}
				if ui.shipHistory == nil {
					ui.shipHistory = shipHistory{}
				}
				for i := 0; i < 2400; i++ {
					at := start.Add(time.Duration(i*100) * time.Second)
					ui.shipHistory[int64(character+1)] = append(ui.shipHistory[int64(character+1)], shipPeriod{CharacterID: int64(character + 1), CharacterName: log.Character, TypeID: 1, TypeName: "Hulk", GroupID: 543, ClassName: "Exhumer", FirstSeen: at, LastSeen: at.Add(89 * time.Second)})
				}
			}
			ui.window.SetContent(ui.content())
			ui.applyGraphRange(ui.snapshot.First, ui.snapshot.Last.Add(time.Second))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if kind == "Zoom" {
					ui.rebuildViewWithOptions(true)
				} else {
					ui.setScopeFilter(graphOutgoing, filterWeaponType, "name:Drone", i%2 == 0)
				}
			}
		})
	}
}
