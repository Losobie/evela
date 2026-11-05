package main

import (
	"container/heap"
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

const engagementIdleGap = 10 * time.Minute

type engagement struct {
	Start, End time.Time // End is exclusive, including the final activity second.
	Summary    string
}

// Merge the already sorted activity streams instead of copying and sorting an
// entire archive's events. Working memory depends on log count and summaries,
// rather than damage-hit count. Chat, travel and other notifications are idle.
type engagementCursor struct {
	log         *parsedLog
	kind, index int
	second      int64
}

const (
	engagementDamage = iota
	engagementEWar
	engagementLogistics
	engagementMining
)

func (cursor *engagementCursor) advance() bool {
	switch cursor.kind {
	case engagementDamage:
		if cursor.index >= len(cursor.log.Hits) {
			return false
		}
		cursor.second = cursor.log.Hits[cursor.index].Second
	case engagementEWar:
		if cursor.index >= len(cursor.log.EWar) {
			return false
		}
		cursor.second = cursor.log.EWar[cursor.index].Second
	case engagementLogistics:
		if cursor.index >= len(cursor.log.Logistics) {
			return false
		}
		cursor.second = cursor.log.Logistics[cursor.index].Second
	case engagementMining:
		if cursor.index >= len(cursor.log.Mining) {
			return false
		}
		cursor.second = cursor.log.Mining[cursor.index].Second
	}
	return true
}

type engagementQueue []engagementCursor

func (queue engagementQueue) Len() int           { return len(queue) }
func (queue engagementQueue) Less(i, j int) bool { return queue[i].second < queue[j].second }
func (queue engagementQueue) Swap(i, j int)      { queue[i], queue[j] = queue[j], queue[i] }
func (queue *engagementQueue) Push(value any)    { *queue = append(*queue, value.(engagementCursor)) }
func (queue *engagementQueue) Pop() any {
	last := len(*queue) - 1
	value := (*queue)[last]
	(*queue)[last] = engagementCursor{}
	*queue = (*queue)[:last]
	return value
}

type engagementSummary struct {
	first, last                             int64
	activities                              [6]bool
	systems, pilots, targets, ores, effects map[string]int
}

func newEngagementSummary(second int64) *engagementSummary {
	return &engagementSummary{first: second, last: second, systems: map[string]int{}, pilots: map[string]int{},
		targets: map[string]int{}, ores: map[string]int{}, effects: map[string]int{}}
}

func (summary *engagementSummary) add(cursor engagementCursor) {
	summary.last = cursor.second
	if cursor.log.Character != "" {
		summary.pilots[cursor.log.Character]++
	}
	switch cursor.kind {
	case engagementDamage:
		hit := cursor.log.Hits[cursor.index]
		if hit.Incoming {
			summary.activities[1] = true
			if hit.Source >= 0 && hit.Source < len(cursor.log.Sources) {
				source := cursor.log.Sources[hit.Source]
				name := source.Character
				if name == "" {
					name = source.Name
				}
				if name != "" {
					summary.targets[name]++
				}
			}
		} else {
			summary.activities[0] = true
			name := hit.Target
			if hit.TargetType != "" && hit.TargetType != name {
				name += " (" + hit.TargetType + ")"
			}
			if name != "" {
				summary.targets[strings.TrimSpace(name)]++
			}
		}
	case engagementEWar:
		summary.activities[2] = true
		if effect := cursor.log.EWar[cursor.index].Effect; effect != "" {
			summary.effects[effect]++
		}
	case engagementLogistics:
		event := cursor.log.Logistics[cursor.index]
		if event.Incoming {
			summary.activities[4] = true
		} else {
			summary.activities[3] = true
		}
	case engagementMining:
		summary.activities[5] = true
		if ore := cursor.log.Mining[cursor.index].Ore; ore != "" {
			summary.ores[ore]++
		}
	}
}

// Prefer the most frequently seen names, breaking ties alphabetically so equal
// timestamps from different log streams always produce the same summary.
func engagementNames(names map[string]int, limit int) string {
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Slice(keys, func(i, j int) bool {
		if names[keys[i]] != names[keys[j]] {
			return names[keys[i]] > names[keys[j]]
		}
		return keys[i] < keys[j]
	})
	text := strings.Join(keys[:min(len(keys), limit)], ", ")
	if len(keys) > limit {
		text += fmt.Sprintf(" +%d more", len(keys)-limit)
	}
	return text
}

func (summary *engagementSummary) finish() engagement {
	system := engagementNames(summary.systems, 3)
	if system == "" {
		system = "Unknown system"
	}
	var activities []string
	for i, name := range []string{"damage dealt", "damage received", "E-War", "outgoing logistics", "incoming logistics", "mining"} {
		if summary.activities[i] {
			activities = append(activities, name)
		}
	}
	parts := []string{system, strings.Join(activities, ", ")}
	if len(summary.pilots) > 0 {
		parts = append(parts, "Pilots: "+engagementNames(summary.pilots, 2))
	}
	if len(summary.targets) > 0 {
		parts = append(parts, "Targets / attackers: "+engagementNames(summary.targets, 2))
	}
	if len(summary.effects) > 0 {
		parts = append(parts, engagementNames(summary.effects, 2))
	}
	if len(summary.ores) > 0 {
		parts = append(parts, "Ore: "+engagementNames(summary.ores, 2))
	}
	return engagement{Start: time.Unix(summary.first, 0).UTC(), End: time.Unix(summary.last+1, 0).UTC(), Summary: strings.Join(parts, " · ")}
}

func buildEngagements(ctx context.Context, snapshot *logSnapshot, locations locationResolver) ([]engagement, error) {
	queue := engagementQueue{}
	for _, log := range snapshot.Logs {
		for kind := engagementDamage; kind <= engagementMining; kind++ {
			cursor := engagementCursor{log: log, kind: kind}
			if cursor.advance() {
				queue = append(queue, cursor)
			}
		}
	}
	heap.Init(&queue)
	var rows []engagement
	var summary *engagementSummary
	// Resolve each character's location only once per activity second, even
	// when hundreds of damage / drone messages arrive in the same second.
	seen := map[string]int64{}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cursor := &queue[0]
		if summary == nil || cursor.second-summary.last >= int64(engagementIdleGap/time.Second) {
			if summary != nil {
				rows = append(rows, summary.finish())
			}
			summary = newEngagementSummary(cursor.second)
			clear(seen)
		}
		summary.add(*cursor)
		if second, ok := seen[cursor.log.Character]; !ok || second != cursor.second {
			seen[cursor.log.Character] = cursor.second
			if location, ok := locations.atName(cursor.log.Character, time.Unix(cursor.second, 0)); ok {
				name := location.SystemName
				if name == "" && location.SystemID > 0 {
					name = fmt.Sprintf("System %d", location.SystemID)
				}
				if name != "" {
					summary.systems[name]++
				}
			}
		}
		cursor.index++
		if cursor.advance() {
			heap.Fix(&queue, 0)
		} else {
			heap.Pop(&queue)
		}
	}
	if summary != nil {
		rows = append(rows, summary.finish())
	}
	slices.Reverse(rows) // Most recent engagements first.
	return rows, ctx.Err()
}
