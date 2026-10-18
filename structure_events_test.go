package main

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"golang.org/x/text/encoding/unicode"
)

const structureHitExample = "[ 2026.10.18 05:46:56 ] (combat) <color=0xff00ffff><b>159</b> <color=0x77ffffff><font size=10>to</font> <b><color=0xffffffff>Valmu - Valmu 2[TLOS](Metenox Moon Drill)</b><font size=10><color=0x77ffffff> - Light Entropic Disintegrator II - Penetrates\n"

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

func TestParseStructureInvulnerabilityContextAndEncoding(t *testing.T) {
	for _, item := range []struct {
		name, contents string
		count          int
	}{
		{"example", structureHitExample + "[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n", 1},
		{"colored notification", structureHitExample + "[ 2026.10.18 05:46:57 ] (notify) <color=red><b>Target is invulnerable.</b></color>\n", 1},
		{"boundary", structureHitExample + "[ 2026.10.18 05:47:26 ] (notify) Target is invulnerable.\n", 1},
		{"expired", structureHitExample + "[ 2026.10.18 05:47:27 ] (notify) Target is invulnerable.\n", 0},
		{"no evidence", "[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n", 0},
		{"ship", "[ 2026.10.18 05:46:56 ] (combat) 159 to Pilot[TEST](Loki) - Laser - Hits\n[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n", 0},
		{"incoming only", strings.Replace(structureHitExample, ">to</font>", ">from</font>", 1) + "[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n", 0},
		{"incoming does not switch target", structureHitExample + "[ 2026.10.18 05:46:57 ] (combat) 30 from Enemy - Laser - Hits\n[ 2026.10.18 05:46:58 ] (notify) Target is invulnerable.\n", 1},
		{"switched to ship", structureHitExample + "[ 2026.10.18 05:46:57 ] (combat) 30 to Pilot[TEST](Loki) - Laser - Hits\n[ 2026.10.18 05:46:58 ] (notify) Target is invulnerable.\n", 0},
		{"switched to NPC", structureHitExample + "[ 2026.10.18 05:46:57 ] (combat) 30 to Guristas Battleship - Laser - Hits\n[ 2026.10.18 05:46:58 ] (notify) Target is invulnerable.\n", 0},
		{"duplicate warnings", structureHitExample + "[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n[ 2026.10.18 05:46:58 ] (notify) Target is invulnerable.\n", 1},
		{"another structure hit rearms", structureHitExample + "[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n" + strings.ReplaceAll(structureHitExample, "05:46:56", "05:46:58") + "[ 2026.10.18 05:46:59 ] (notify) Target is invulnerable.\n", 2},
		{"backwards notification", structureHitExample + "[ 2026.10.18 05:46:55 ] (notify) Target is invulnerable.\n", 0},
		{"new listener", structureHitExample + "Listener: Other Pilot\n[ 2026.10.18 05:46:57 ] (notify) Target is invulnerable.\n", 0},
	} {
		for _, encoding := range []string{"utf8", "utf16"} {
			t.Run(item.name+"/"+encoding, func(t *testing.T) {
				data := []byte("Listener: Pilot\n" + item.contents)
				if encoding == "utf16" {
					var err error
					data, err = unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(data)
					if err != nil {
						t.Fatal(err)
					}
				}
				parsed, err := parseLog(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				if len(parsed.StructureEvents) != item.count {
					t.Fatalf("got structure events %+v, want %d", parsed.StructureEvents, item.count)
				}
				if item.count > 0 && (parsed.StructureEvents[0].Type != "Metenox Moon Drill" || !strings.Contains(parsed.StructureEvents[0].Target, "Valmu - Valmu 2[TLOS]")) {
					t.Fatal("structure target identity was lost")
				}
			})
		}
	}
}

func TestStructureEventExtendsLoadedTimelineAndRefreshes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "20261018_054600_1.txt")
	contents := "Listener: Pilot\n" + structureHitExample
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	index := &logIndex{}
	before, err := index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	contents += "[ 2026.10.18 05:46:58 ] (notify) Target is invulnerable.\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := index.refresh(context.Background(), directory, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Last.Sub(before.Last) != 2*time.Second || len(after.Logs[0].StructureEvents) != 1 || after.Logs[0].Points[0].Outgoing != 159 || after.Logs[0].Points[0].OutgoingHits != 1 {
		t.Fatal("appended notification was missed, changed damage totals or failed to extend the timeline")
	}
	start, end := presetRange("Last 30 seconds", after)
	view := buildView(after, start, end, time.Second)
	if len(view.structureMarkers(after, map[string]bool{"Pilot": true})) != 1 {
		t.Fatal("relative range excluded a marker after the last damage hit")
	}
}

func TestStructureMarkersOutgoingRenderingSelectionAndFormats(t *testing.T) {
	application := test.NewTempApp(t)
	window := application.NewWindow("Structure invulnerability marker")
	defer window.Close()
	ui := newAnalyzerUI(window)
	start := time.Date(2026, 10, 18, 5, 46, 54, 0, time.UTC)
	for _, name := range []string{"A", "B"} {
		parsed, err := parseLog(strings.NewReader("Listener: " + name + "\n" + structureHitExample + "[ 2026.10.18 05:46:58 ] (notify) Target is invulnerable.\n"))
		if err != nil {
			t.Fatal(err)
		}
		ui.snapshot.Characters = append(ui.snapshot.Characters, name)
		ui.snapshot.Logs = append(ui.snapshot.Logs, parsed)
		ui.selected[name] = true
	}
	ui.applyGraphRange(start, start.Add(10*time.Second))
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.Show()
	red := color.RGBA{255, 70, 70, 255}
	for _, grouping := range []string{"Characters", "Ship history", "Ship class history"} {
		ui.groupSelect.SetSelected(grouping)
		ui.graphAverage10s, ui.graphLogarithmic = true, true
		ui.renderSelection()
		if len(ui.outgoing.data.Markers) != 1 || len(ui.incoming.data.Markers) != 0 {
			t.Fatal("duplicate pilots produced multiple markers or marked the incoming graph")
		}
		plot := ui.outgoing.renderPlot(101, 51)
		for _, y := range []int{0, 25, 50} {
			if plot.At(40, y) != red {
				t.Fatalf("%s: marker was not a full-height red line at the event time", grouping)
			}
		}
		width := ui.outgoing.Size().Width - chartPlotLeft - chartPlotRight
		ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+width*.4, 80, 0))
		if !strings.Contains(ui.outgoing.hover.Text, "Target invulnerable at 05:46:58 UTC") || !strings.Contains(ui.outgoing.hover.Text, "Metenox Moon Drill") {
			t.Fatalf("marker hover did not identify the event: %s", ui.outgoing.hover.Text)
		}
	}
	if path := os.Getenv("EVELA_STRUCTURE_CAPTURE"); path != "" {
		ui.outgoing.MouseOut()
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, window.Canvas().Capture())
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	ui.applyGraphRange(start, start.Add(4*time.Second))
	if len(ui.outgoing.data.Markers) != 0 {
		t.Fatal("marker at the exclusive range end was not clipped")
	}
	ui.applyGraphRange(start.Add(3*time.Second), start.Add(5*time.Second))
	if len(ui.outgoing.data.Markers) != 1 {
		t.Fatal("zooming to the notification without its earlier hit hid the marker")
	}
	width := ui.outgoing.Size().Width - chartPlotLeft - chartPlotRight
	ui.outgoing.MouseMoved(testMouseEvent(chartPlotLeft+width/2, 80, 0))
	if !strings.Contains(ui.outgoing.hover.Text, "Target invulnerable") {
		t.Fatal("a notification-only range could not inspect its marker")
	}
	ui.selected["A"], ui.selected["B"] = false, false
	ui.renderSelection()
	if len(ui.outgoing.data.Markers) != 0 {
		t.Fatal("unselected characters retained their markers")
	}
}
