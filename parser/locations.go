package parser

import (
	"regexp"
	"strings"
)

var (
	logJump             = regexp.MustCompile(`^Jumping from (.+?) to (.+?)\.?$`)
	logConduitPassenger = regexp.MustCompile(`^A Conduit Field activated by .+ jumps you to (.+)\.$`)
	logConduitPilot     = regexp.MustCompile(`^Activating a Conduit Field jumps you to (.+), bringing along [0-9]+ passengers?\.$`)
)

const SpatialPhenomenaMessage = "Local spatial phenomena may cause strange effects on your ship systems."

type LocationEvent struct {
	Second   int64
	From     string
	To       string
	Inferred bool `json:",omitempty"`
}

func ParseLocation(message string, second int64) (LocationEvent, bool) {
	event := LocationEvent{Second: second}
	if match := logJump.FindStringSubmatch(message); match != nil {
		event.From, event.To = strings.TrimSpace(match[1]), strings.TrimSpace(match[2])
	} else if match := logConduitPassenger.FindStringSubmatch(message); match != nil {
		event.To = strings.TrimSpace(match[1])
	} else if match := logConduitPilot.FindStringSubmatch(message); match != nil {
		event.To = strings.TrimSpace(match[1])
	} else if message == SpatialPhenomenaMessage {
		event.To, event.Inferred = "Wormhole", true
	}
	return event, event.To != ""
}
