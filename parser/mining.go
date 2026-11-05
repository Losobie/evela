package parser

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	Mined      = "Ore mined"
	Compressed = "Compressed"
	Waste      = "Waste"
)

var minedMessage = regexp.MustCompile(`^You mined ([0-9][0-9,]*) units of (.+?)(?: with a lost residue of ([0-9][0-9,]*) units)?\.?$`)
var compressedMessage = regexp.MustCompile(`^Successfully compressed (.+?) into ([0-9][0-9,]*) (.+)\.$`)

type MiningEvent struct {
	Second    int64
	Ore, Kind string
	Amount    float64
	Message   string
}

func miningAmount(text string) (float64, bool) {
	// Parse integers first so malformed separators and overflowing quantities
	// cannot silently become misleading graph values.
	if strings.Contains(text, ",") {
		parts := strings.Split(text, ",")
		if len(parts[0]) < 1 || len(parts[0]) > 3 {
			return 0, false
		}
		for _, part := range parts[1:] {
			if len(part) != 3 {
				return 0, false
			}
		}
	}
	amount, err := strconv.ParseUint(strings.ReplaceAll(text, ",", ""), 10, 64)
	return float64(amount), err == nil
}

func ParseMining(message, channel string, second int64) []MiningEvent {
	if channel == "(mining)" {
		match := minedMessage.FindStringSubmatch(message)
		if match == nil {
			return nil
		}
		amount, ok := miningAmount(match[1])
		if !ok {
			return nil
		}
		ore := strings.TrimSpace(match[2])
		if ore == "" || strings.Contains(ore, " with a lost residue") {
			return nil
		}
		events := []MiningEvent{{Second: second, Ore: strings.Clone(ore), Kind: Mined, Amount: amount, Message: strings.Clone(message)}}
		if match[3] != "" {
			waste, ok := miningAmount(match[3])
			if !ok {
				return nil
			}
			events = append(events, MiningEvent{Second: second, Ore: events[0].Ore, Kind: Waste, Amount: waste, Message: events[0].Message})
		}
		return events
	}
	if channel == "(notify)" {
		match := compressedMessage.FindStringSubmatch(message)
		if match == nil {
			return nil
		}
		amount, ok := miningAmount(match[2])
		ore := strings.TrimSpace(match[1])
		if !ok || ore == "" || !strings.EqualFold(strings.TrimSpace(match[3]), "Compressed "+ore) {
			return nil
		}
		return []MiningEvent{{Second: second, Ore: strings.Clone(ore), Kind: Compressed, Amount: amount, Message: strings.Clone(message)}}
	}
	return nil
}
