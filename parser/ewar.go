package parser

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type EWarEvent struct {
	Second  int64
	Effect  string
	Amount  float64 // GJ lost; only capacitor warfare has a numeric magnitude.
	Message string
}

var (
	capacitorWarfareMessage = regexp.MustCompile(`(?i)^([+-]?[0-9][0-9,.]*)\s+GJ\s+energy\s+(neutralized|neutralised|drained)\s+(.+)$`)
	ewarAttemptMessage      = regexp.MustCompile(`(?i)^(.+?) attempt from (.+) to you[!.]?$`)
	ewarReceivedMessage     = regexp.MustCompile(`(?i)^You (?:are|have been) (?:being )?(.+?)(?: by .+)?[!.]?$`)
)

// Recognize incoming messages, never module names mentioned in damage, chat,
// activation errors or outgoing notifications. A log entry is an event, not
// evidence of effect duration, strength or (for attempts) successful application.
func ParseIncomingEWar(message string, second int64) (EWarEvent, bool) {
	event := EWarEvent{Second: second, Message: strings.Clone(message)}
	if match := capacitorWarfareMessage.FindStringSubmatch(message); match != nil {
		amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
			return EWarEvent{}, false
		}
		direction := strings.ToLower(match[3])
		if strings.EqualFold(match[2], "drained") {
			// NOS gains are '+N ... drained from'; losses are '-N ... drained to'.
			if !strings.HasPrefix(direction, "to ") || strings.HasPrefix(match[1], "+") || amount > 0 {
				return EWarEvent{}, false
			}
			event.Effect = "Nosferatu"
		} else {
			// The victim's neutralizer message has an unsigned amount and source.
			if amount < 0 || strings.HasPrefix(match[1], "+") || strings.HasPrefix(direction, "to ") || strings.HasPrefix(direction, "by you") {
				return EWarEvent{}, false
			}
			event.Effect = "Energy neutralizer"
		}
		event.Amount = math.Abs(amount)
		return event, true
	}
	var action string
	if match := ewarAttemptMessage.FindStringSubmatch(message); match != nil {
		action = match[1]
	} else if match := ewarReceivedMessage.FindStringSubmatch(message); match != nil {
		action = match[1]
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "warp scramble", "warp scrambled":
		event.Effect = "Warp scramble"
	case "warp disruption", "warp disrupted":
		event.Effect = "Warp disruption"
	case "stasis webifying", "stasis webification", "webifying", "webified":
		event.Effect = "Stasis webifier"
	case "target painting", "target painted":
		event.Effect = "Target painter"
	case "tracking disruption", "tracking disrupted", "turret disruption", "turret disrupted":
		event.Effect = "Turret disruption"
	case "guidance disruption", "guidance disrupted", "missile disruption", "missile disrupted":
		event.Effect = "Missile disruption"
	case "sensor dampening", "sensor dampened", "remote sensor dampening":
		event.Effect = "Sensor dampening"
	case "sensor jam", "sensor jamming", "electronic counter measures", "jammed":
		event.Effect = "ECM"
	default:
		return EWarEvent{}, false
	}
	return event, true
}
