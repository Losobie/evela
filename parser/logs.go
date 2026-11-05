package parser

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"html"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const TimeLayout = "2006.01.02 15:04:05"

var (
	markup        = regexp.MustCompile(`<[^>]*>`)
	damageMessage = regexp.MustCompile(`^([0-9][0-9,.]*)\s+(from|to)\s+(\S.*)$`)
)

type DamagePoint struct {
	Second                     int64
	Incoming, Outgoing         float64
	IncomingHits, OutgoingHits int
}

type DamageHit struct {
	Second                                                     int64
	Amount                                                     float64
	Source                                                     int
	Incoming                                                   bool
	WeaponType, WeaponStrength                                 string
	Target, TargetType                                         string
	TargetLocation, TargetCorporation, TargetCorporationTicker string
	DeflectionAmount                                           float64
	HasDeflection                                              bool
}

type Log struct {
	Character       string
	Points          []DamagePoint
	Hits            []DamageHit
	Sources         []DamageSource
	IncomingSources []IncomingSourcePoint
	StructureEvents []StructureEvent
	EntrySeconds    []int64
	Events          []Event
	EWar            []EWarEvent
	Logistics       []LogisticsEvent
	Mining          []MiningEvent
	SessionStarted  time.Time
	LocationEvents  []LocationEvent
	Hash            [32]byte
}

// Parse accepts EVE's UTF-8 and BOM-marked UTF-16 exports. Rates use numeric
// damage messages; logistics, E-War and structure notifications are separate.
func Parse(input io.Reader) (*Log, error) {
	hash := sha256.New()
	raw := bufio.NewReader(io.TeeReader(input, hash))
	var reader io.Reader = raw
	if bom, _ := raw.Peek(2); bytes.Equal(bom, []byte{0xff, 0xfe}) || bytes.Equal(bom, []byte{0xfe, 0xff}) {
		reader = transform.NewReader(raw, unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder())
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	result := &Log{}
	seconds := make(map[int64]*DamagePoint)
	sourceIDs := make(map[string]int)
	sourceNames := make(map[DamageSource]int)
	sourceSeconds := make(map[struct {
		second int64
		source int
	}]*IncomingSourcePoint)
	entries := make(map[int64]bool)
	var lastStructure StructureEvent
	structurePending := false
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if strings.HasPrefix(line, "Listener:") {
			result.Character = strings.TrimSpace(strings.TrimPrefix(line, "Listener:"))
			structurePending = false
			continue
		}
		if strings.HasPrefix(line, "Session Started:") {
			result.SessionStarted, _ = time.Parse(TimeLayout, strings.TrimSpace(strings.TrimPrefix(line, "Session Started:")))
			continue
		}
		if len(line) < 25 || line[0] != '[' {
			continue
		}
		closeBracket := strings.IndexByte(line, ']')
		if closeBracket < 0 {
			continue
		}
		at, err := time.Parse(TimeLayout, strings.TrimSpace(line[1:closeBracket]))
		if err != nil {
			continue
		}
		entries[at.Unix()] = true
		message := strings.TrimSpace(line[closeBracket+1:])
		message = strings.TrimSpace(html.UnescapeString(markup.ReplaceAllString(message, "")))
		result.Events = append(result.Events, Event{Second: at.Unix(), Message: strings.Clone(message)})
		channelEnd := strings.IndexByte(message, ')')
		if !strings.HasPrefix(message, "(") || channelEnd < 0 {
			continue
		}
		combat := message[:channelEnd+1] == "(combat)"
		channel := message[:channelEnd+1]
		if channel == "(mining)" || channel == "(notify)" {
			if events := ParseMining(strings.TrimSpace(message[channelEnd+1:]), channel, at.Unix()); len(events) > 0 {
				result.Mining = append(result.Mining, events...)
				continue
			}
		}
		locationChannel := channel == "(notify)" || channel == "(None)" || channel == "(info)"
		if !combat && !locationChannel && !strings.Contains(message, "invulnerable") {
			continue
		}
		message = strings.TrimSpace(message[channelEnd+1:])
		if combat || locationChannel {
			if combat {
				if event, ok := ParseLogistics(message, at.Unix()); ok {
					result.Logistics = append(result.Logistics, event)
					continue
				}
			}
			if event, ok := ParseIncomingEWar(message, at.Unix()); ok {
				result.EWar = append(result.EWar, event)
				continue
			}
		}
		if locationChannel {
			if event, ok := ParseLocation(message, at.Unix()); ok {
				result.LocationEvents = append(result.LocationEvents, event)
				if !event.Inferred {
					structurePending = false // A target in the old system cannot carry over.
				}
				continue
			}
		}
		if message == "Target is invulnerable." {
			if structurePending && at.Unix() >= lastStructure.Second && at.Unix()-lastStructure.Second <= int64(structureInvulnerabilityWindow/time.Second) {
				event := lastStructure
				event.Second = at.Unix()
				result.StructureEvents = append(result.StructureEvents, event)
				structurePending = false // One marker until another structure hit.
			}
			continue
		}
		if !combat {
			continue
		}
		match := damageMessage.FindStringSubmatch(message)
		if match == nil {
			if target, weapon, incoming, ok := parseDamageMiss(message); ok {
				sourceID := -1
				if incoming {
					source := IncomingDamageSource(target)
					var exists bool
					sourceID, exists = sourceNames[source]
					if !exists {
						sourceID = len(result.Sources)
						result.Sources = append(result.Sources, source)
						sourceNames[source] = sourceID
					}
				}
				hit := DamageHit{Second: at.Unix(), Source: sourceID, Incoming: incoming, WeaponType: weapon, WeaponStrength: "Misses"}
				if !incoming {
					hit.setTarget(ParseCombatEntity(target))
				}
				result.Hits = append(result.Hits, hit)
			}
			continue
		}
		amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
		if err != nil || amount < 0 || math.IsInf(amount, 0) || math.IsNaN(amount) {
			continue
		}
		detail, deflection, hasDeflection := damageWithoutDeflection(match[3])
		weapon, strength := damageWeaponDetails(detail)
		second := at.Unix()
		point := seconds[second]
		if point == nil {
			point = &DamagePoint{Second: second}
			seconds[second] = point
		}
		if match[2] == "from" {
			point.Incoming += amount
			point.IncomingHits++
			sourceID, exists := sourceIDs[match[3]]
			if !exists {
				source := IncomingDamageSource(detail)
				// Different weapons and hit qualities still belong to one source.
				sourceID, exists = sourceNames[source]
				if !exists {
					sourceID = len(result.Sources)
					result.Sources = append(result.Sources, source)
					sourceNames[source] = sourceID
				}
				sourceIDs[strings.Clone(match[3])] = sourceID
			}
			key := struct {
				second int64
				source int
			}{second, sourceID}
			contribution := sourceSeconds[key]
			if contribution == nil {
				contribution = &IncomingSourcePoint{Second: second, Source: sourceID}
				sourceSeconds[key] = contribution
			}
			contribution.Damage += amount
			contribution.Hits++
			result.Hits = append(result.Hits, DamageHit{Second: second, Amount: amount, Source: sourceID, Incoming: true, WeaponType: weapon, WeaponStrength: strength, DeflectionAmount: deflection, HasDeflection: hasDeflection})
		} else {
			point.Outgoing += amount
			point.OutgoingHits++
			hit := DamageHit{Second: second, Amount: amount, Source: -1, WeaponType: weapon, WeaponStrength: strength, DeflectionAmount: deflection, HasDeflection: hasDeflection}
			hit.setTarget(damageTarget(detail, weapon, strength))
			result.Hits = append(result.Hits, hit)
			target, typeName, recognized := outgoingStructureTarget(detail)
			structurePending = recognized
			if recognized {
				lastStructure = StructureEvent{Second: second, Target: target, Type: typeName}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	copy(result.Hash[:], hash.Sum(nil))
	for second := range entries {
		result.EntrySeconds = append(result.EntrySeconds, second)
	}
	sort.Slice(result.EntrySeconds, func(i, j int) bool { return result.EntrySeconds[i] < result.EntrySeconds[j] })
	sort.SliceStable(result.Events, func(i, j int) bool { return result.Events[i].Second < result.Events[j].Second })
	for _, point := range seconds {
		result.Points = append(result.Points, *point)
	}
	sort.Slice(result.Points, func(i, j int) bool { return result.Points[i].Second < result.Points[j].Second })
	sort.SliceStable(result.Hits, func(i, j int) bool { return result.Hits[i].Second < result.Hits[j].Second })
	sort.SliceStable(result.EWar, func(i, j int) bool { return result.EWar[i].Second < result.EWar[j].Second })
	sort.SliceStable(result.Logistics, func(i, j int) bool { return result.Logistics[i].Second < result.Logistics[j].Second })
	sort.SliceStable(result.Mining, func(i, j int) bool { return result.Mining[i].Second < result.Mining[j].Second })
	for _, point := range sourceSeconds {
		result.IncomingSources = append(result.IncomingSources, *point)
	}
	sort.Slice(result.IncomingSources, func(i, j int) bool {
		a, b := result.IncomingSources[i], result.IncomingSources[j]
		return a.Second < b.Second || (a.Second == b.Second && a.Source < b.Source)
	})
	sort.Slice(result.StructureEvents, func(i, j int) bool { return result.StructureEvents[i].Second < result.StructureEvents[j].Second })
	return result, nil
}
