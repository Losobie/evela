package main

import (
	"container/heap"
	"context"
	"fmt"
	"slices"
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

func (queue *engagementQueue) advance() {
	cursor := &(*queue)[0]
	cursor.index++
	if cursor.advance() {
		heap.Fix(queue, 0)
	} else {
		heap.Pop(queue)
	}
}

func (summary *engagementSummary) finish() engagement {
	return engagement{Start: time.Unix(summary.first, 0).UTC(), End: time.Unix(summary.last+1, 0).UTC(), Summary: summary.text()}
}

func buildEngagements(ctx context.Context, snapshot *logSnapshot, locations locationResolver, ships shipHistory) ([]engagement, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	identities := newEngagementIdentities(ships)
	shipNames := ships.indexByName()
	actorShips := map[string]engagementIdentity{}
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
		if cursor.kind == engagementLogistics {
			event := cursor.log.Logistics[cursor.index]
			// Empty reps and cap chains do not establish activity, bridge idle
			// gaps, extend ranges, or introduce systems / ships into summaries.
			if event.Amount <= 0 || event.Kind == "Capacitor transfer" {
				queue.advance()
				continue
			}
		}
		if summary == nil || cursor.second-summary.last >= int64(engagementIdleGap/time.Second) {
			if summary != nil {
				rows = append(rows, summary.finish())
			}
			summary = newEngagementSummary(cursor.second)
			clear(seen)
		}
		if second, ok := seen[cursor.log.Character]; !ok || second != cursor.second {
			seen[cursor.log.Character] = cursor.second
			ship, _ := shipNames[characterKey(cursor.log.Character)].atName(cursor.log.Character, time.Unix(cursor.second, 0))
			actorShips[cursor.log.Character] = identities.hull(ship.TypeName)
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
		summary.add(*cursor, actorShips[cursor.log.Character], identities)
		queue.advance()
	}
	if summary != nil {
		rows = append(rows, summary.finish())
	}
	slices.Reverse(rows) // Most recent engagements first.
	return rows, ctx.Err()
}
