package parser

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type LogisticsEvent struct {
	Second                                                     int64
	Kind                                                       string
	Amount                                                     float64
	Incoming                                                   bool
	Message                                                    string
	Target, TargetType                                         string
	TargetLocation, TargetCorporation, TargetCorporationTicker string
	TargetAllianceTicker                                       string
}

// Incoming remote assistance uses "by" (also accept explicit "from");
// outgoing assistance uses "to". Require a remote-assistance verb so local
// tanking, capacitor boosters, NOS and neutralizers cannot become logistics.
var logisticsMessage = regexp.MustCompile(`(?i)^([0-9][0-9,.]*)(?:\s+(?:HP|GJ))?\s+remote\s+(armor repaired|shield boosted|capacitor transmitted)\s+(to|by|from)\s+(\S.*)$`)

func ParseLogistics(message string, second int64) (LogisticsEvent, bool) {
	match := logisticsMessage.FindStringSubmatch(message)
	if match == nil {
		return LogisticsEvent{}, false
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return LogisticsEvent{}, false
	}
	kind := "Capacitor transfer"
	switch strings.ToLower(match[2]) {
	case "armor repaired":
		kind = "Armor repair"
	case "shield boosted":
		kind = "Shield repair"
	}
	event := LogisticsEvent{Second: second, Kind: kind, Amount: amount, Incoming: !strings.EqualFold(match[3], "to"), Message: strings.Clone(message)}
	if !event.Incoming {
		entity, alliance := logisticsTargetDetails(match[4])
		event.Target, event.TargetType, event.TargetLocation = entity.Name, entity.Type, entity.Location
		event.TargetCorporation, event.TargetCorporationTicker = entity.Corporation, entity.CorporationTicker
		event.TargetAllianceTicker = alliance
	}
	return event, true
}
