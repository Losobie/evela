package parser_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/Losobie/evela/parser"
	"golang.org/x/text/encoding/unicode"
)

func ExampleParse() {
	log, err := parser.Parse(strings.NewReader("Listener: Pilot\n[ 2026.10.19 02:00:00 ] (combat) 50 to Target - Laser - Hits\n"))
	if err != nil {
		panic(err)
	}
	fmt.Println(log.Character, log.Hits[0].Amount, log.Hits[0].WeaponType)
	// Output: Pilot 50 Laser
}

func TestStandaloneParseAcrossEncodings(t *testing.T) {
	source := "Listener: Pilot\n" +
		"[ 2026.10.19 02:00:00 ] (combat) 60 to Orbital Skyhook (R3-K7K IX) [Unfinished Symphony][MOVMT](Orbital Skyhook) - Vespa II - Glances Off - Glances Off (60 deflected)\n" +
		"[ 2026.10.19 02:00:01 ] (combat) Blood Raiders Apocalypse misses you completely\n" +
		"[ 2026.10.19 02:00:02 ] (combat) 472 remote shield boosted to Customs Office (R3-K7K IX) [Way So Mad] - Large Remote Shield Booster II\n" +
		"[ 2026.10.19 02:00:03 ] (combat) 100 GJ energy neutralized Enemy\n" +
		"[ 2026.10.19 02:00:04 ] (mining) You mined 10 units of Xenotime with a lost residue of 2 units.\n" +
		"[ 2026.10.19 02:00:05 ] (notify) Jumping from Mamet to Hoshoun\n"
	for _, encoding := range []string{"utf8", "utf16le", "utf16be"} {
		t.Run(encoding, func(t *testing.T) {
			input := []byte(source)
			if encoding != "utf8" {
				order := unicode.LittleEndian
				if encoding == "utf16be" {
					order = unicode.BigEndian
				}
				var err error
				input, err = unicode.UTF16(order, unicode.UseBOM).NewEncoder().Bytes(input)
				if err != nil {
					t.Fatal(err)
				}
			}
			log, err := parser.Parse(bytes.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			if log.Hash != sha256.Sum256(input) || log.Character != "Pilot" || len(log.Events) != 6 || len(log.Hits) != 2 || len(log.Logistics) != 1 || len(log.EWar) != 1 || len(log.Mining) != 2 || len(log.LocationEvents) != 1 {
				t.Fatalf("incomplete standalone parse: %+v", log)
			}
			hit := log.Hits[0]
			if hit.TargetType != "Orbital Skyhook" || hit.TargetLocation != "R3-K7K IX" || hit.TargetCorporationTicker != "MOVMT" || !hit.HasDeflection || hit.DeflectionAmount != 60 || hit.Amount != 60 {
				t.Fatalf("damage metadata changed: %+v", hit)
			}
			if log.Hits[1].WeaponStrength != "Misses" || log.Logistics[0].TargetType != "Customs Office" || log.LocationEvents[0].To != "Hoshoun" {
				t.Fatal("activity details changed")
			}
			first, last := log.ActivityBounds()
			if first.Format("15:04:05") != "02:00:00" || last.Format("15:04:05") != "02:00:04" {
				t.Fatalf("travel incorrectly extended activity bounds: %v to %v", first, last)
			}
		})
	}
}
