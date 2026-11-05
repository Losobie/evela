package main

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"
)

type extractedEventField struct{ Name, Value string }

func (viewer *eventsViewer) eventDetails(event eventRow) string {
	ship, system := "Unknown ship", "Unknown system"
	if viewer.contextAt != nil {
		ship, system = viewer.contextAt(event.Character, event.Second)
	}
	text := time.Unix(event.Second, 0).UTC().Format("2006-01-02 15:04:05") + " EVE time\nCharacter: " + event.Character + "\n\n" + event.Message +
		"\n\nCurrent ship: " + ship + "\nCurrent system: " + system + "\n\nExtracted fields"
	return text + "\n" + strings.TrimSuffix(formatExtractedFields(viewer.eventFields(event)), "\n")
}

// Parse just the selected message through the real log parser. This preserves
// channel restrictions, miss / deflection handling and source classification,
// without adding work or a field index to every event during loading / zooming.
func (viewer *eventsViewer) eventFields(event eventRow) []extractedEventField {
	var fields []extractedEventField
	add := func(name, value string) {
		if value != "" {
			fields = append(fields, extractedEventField{name, value})
		}
	}
	number := func(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
	direction := func(incoming bool) string {
		if incoming {
			return "Incoming"
		}
		return "Outgoing"
	}
	entity := func(prefix string, name, typ, location, corporation, ticker string) {
		add(prefix, name)
		add(prefix+" type", typ)
		add(prefix+" location", location)
		add(prefix+" corporation", corporation)
		add(prefix+" corporation ticker", ticker)
	}
	if end := strings.IndexByte(event.Message, ')'); strings.HasPrefix(event.Message, "(") && end >= 0 {
		add("Channel", event.Message[1:end])
	}
	// Stored event text is already decoded. Escape it so the parser's markup
	// normalization preserves literal angle brackets and entity-like names.
	line := "[ " + time.Unix(event.Second, 0).UTC().Format(logTimeLayout) + " ] " + html.EscapeString(event.Message)
	parsed, err := parseLog(strings.NewReader(line))
	if err != nil {
		add("Parsing error", err.Error())
		return fields
	}
	for _, hit := range parsed.Hits {
		add("Activity", "Damage")
		add("Direction", direction(hit.Incoming))
		add("Amount", number(hit.Amount))
		add("Weapon type", hit.WeaponType)
		add("Weapon strength", hit.WeaponStrength)
		entity("Target", hit.Target, hit.TargetType, hit.TargetLocation, hit.TargetCorporation, hit.TargetCorporationTicker)
		if hit.HasDeflection {
			add("Deflection amount", number(hit.DeflectionAmount))
		}
		if hit.Incoming && hit.Source >= 0 && hit.Source < len(parsed.Sources) {
			source := parsed.Sources[hit.Source]
			add("Attacker", source.Name)
			add("Attacker character", source.Character)
			add("Attacker type", source.Hull)
			add("Attacker location", source.Location)
			add("Attacker corporation", source.Corporation)
			add("Attacker corporation ticker", source.CorporationTicker)
			add("Attacker NPC", strconv.FormatBool(source.NPC))
			if source.TypeID != 0 {
				add("Attacker type ID", strconv.Itoa(source.TypeID))
			}
			if source.GroupID != 0 {
				add("Attacker group ID", strconv.Itoa(source.GroupID))
			}
		}
	}
	for _, logistics := range parsed.Logistics {
		add("Activity", "Logistics")
		add("Direction", direction(logistics.Incoming))
		add("Logistics type", logistics.Kind)
		add("Amount", number(logistics.Amount))
		entity("Target", logistics.Target, logistics.TargetType, logistics.TargetLocation, logistics.TargetCorporation, logistics.TargetCorporationTicker)
		add("Target alliance ticker", logistics.TargetAllianceTicker)
	}
	for _, ewar := range parsed.EWar {
		add("Activity", "E-War")
		add("Direction", "Incoming")
		add("Effect", ewar.Effect)
		if ewar.Effect == "Energy neutralizer" || ewar.Effect == "Nosferatu" {
			add("Capacitor drained (GJ)", number(ewar.Amount))
		}
	}
	for _, mining := range parsed.Mining {
		add("Activity", "Mining")
		add("Message type", mining.Kind)
		add("Ore", mining.Ore)
		add("Amount", number(mining.Amount))
	}
	for _, location := range parsed.LocationEvents {
		if location.Inferred {
			add("Activity", "Spatial phenomena")
			add("Location hint", location.To)
			continue
		}
		add("Activity", "Travel")
		add("From system", location.From)
		add("To system", location.To)
	}
	// Invulnerability is contextual: the parser associates a preceding structure
	// hit with this notification. Read those stored fields from the original log,
	// using a row index rather than keeping old parsed logs alive in hidden rows.
	if strings.HasSuffix(event.Message, ") Target is invulnerable.") && viewer.snapshot != nil && event.logIndex >= 0 && event.logIndex < len(viewer.snapshot.Logs) {
		markers := viewer.snapshot.Logs[event.logIndex].StructureEvents
		first := sort.Search(len(markers), func(i int) bool { return markers[i].Second >= event.Second })
		for _, marker := range markers[first:] {
			if marker.Second != event.Second {
				break
			}
			add("Activity", "Structure invulnerability")
			add("Target", marker.Target)
			add("Target type", marker.Type)
		}
	}
	if len(parsed.Hits)+len(parsed.Logistics)+len(parsed.EWar)+len(parsed.Mining)+len(parsed.LocationEvents) == 0 && len(fields) <= 1 {
		add("Activity fields", "None extracted")
	}
	return fields
}

// Keep field formatting separate from the widgets so details can be checked
// against cached parsing, even for multiple different events in one second.
func formatExtractedFields(fields []extractedEventField) string {
	var text strings.Builder
	for _, field := range fields {
		fmt.Fprintf(&text, "%s: %s\n", field.Name, field.Value)
	}
	return text.String()
}
