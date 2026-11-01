package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const logTimeLayout = "2006.01.02 15:04:05"

var (
	markup        = regexp.MustCompile(`<[^>]*>`)
	damageMessage = regexp.MustCompile(`^([0-9][0-9,.]*)\s+(from|to)\s+(\S.*)$`)
)

type damagePoint struct {
	Second                     int64
	Incoming, Outgoing         float64
	IncomingHits, OutgoingHits int
}

type damageHit struct {
	Second                     int64
	Amount                     float64
	Source                     int
	Incoming                   bool
	WeaponType, WeaponStrength string
}

type parsedLog struct {
	Character       string
	Points          []damagePoint
	Hits            []damageHit
	Sources         []damageSource
	IncomingSources []incomingSourcePoint
	StructureEvents []structureEvent
	EntrySeconds    []int64
	Events          []logEvent
	EWar            []ewarEvent
	Logistics       []logisticsEvent
	Mining          []miningEvent
	SessionStarted  time.Time
	LocationEvents  []logLocationEvent
	Hash            [32]byte
}

// parseLog accepts EVE's UTF-8 and BOM-marked UTF-16 exports. Rates use numeric
// damage messages; logistics, E-War and structure notifications are separate.
func parseLog(input io.Reader) (*parsedLog, error) {
	hash := sha256.New()
	raw := bufio.NewReader(io.TeeReader(input, hash))
	var reader io.Reader = raw
	if bom, _ := raw.Peek(2); bytes.Equal(bom, []byte{0xff, 0xfe}) || bytes.Equal(bom, []byte{0xfe, 0xff}) {
		reader = transform.NewReader(raw, unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder())
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	result := &parsedLog{}
	seconds := make(map[int64]*damagePoint)
	sourceIDs := make(map[string]int)
	sourceNames := make(map[damageSource]int)
	sourceSeconds := make(map[struct {
		second int64
		source int
	}]*incomingSourcePoint)
	entries := make(map[int64]bool)
	var lastStructure structureEvent
	structurePending := false
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if strings.HasPrefix(line, "Listener:") {
			result.Character = strings.TrimSpace(strings.TrimPrefix(line, "Listener:"))
			structurePending = false
			continue
		}
		if strings.HasPrefix(line, "Session Started:") {
			result.SessionStarted, _ = time.Parse(logTimeLayout, strings.TrimSpace(strings.TrimPrefix(line, "Session Started:")))
			continue
		}
		if len(line) < 25 || line[0] != '[' {
			continue
		}
		closeBracket := strings.IndexByte(line, ']')
		if closeBracket < 0 {
			continue
		}
		at, err := time.Parse(logTimeLayout, strings.TrimSpace(line[1:closeBracket]))
		if err != nil {
			continue
		}
		entries[at.Unix()] = true
		message := strings.TrimSpace(line[closeBracket+1:])
		message = strings.TrimSpace(html.UnescapeString(markup.ReplaceAllString(message, "")))
		result.Events = append(result.Events, logEvent{Second: at.Unix(), Message: strings.Clone(message)})
		channelEnd := strings.IndexByte(message, ')')
		if !strings.HasPrefix(message, "(") || channelEnd < 0 {
			continue
		}
		combat := message[:channelEnd+1] == "(combat)"
		channel := message[:channelEnd+1]
		if channel == "(mining)" || channel == "(notify)" {
			if events := parseMining(strings.TrimSpace(message[channelEnd+1:]), channel, at.Unix()); len(events) > 0 {
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
				if event, ok := parseLogistics(message, at.Unix()); ok {
					result.Logistics = append(result.Logistics, event)
					continue
				}
			}
			if event, ok := parseIncomingEWar(message, at.Unix()); ok {
				result.EWar = append(result.EWar, event)
				continue
			}
		}
		if locationChannel {
			if event, ok := parseLogLocation(message, at.Unix()); ok {
				result.LocationEvents = append(result.LocationEvents, event)
				structurePending = false // A target in the old system cannot carry over.
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
					source := incomingDamageSource(target)
					var exists bool
					sourceID, exists = sourceNames[source]
					if !exists {
						sourceID = len(result.Sources)
						result.Sources = append(result.Sources, source)
						sourceNames[source] = sourceID
					}
				}
				result.Hits = append(result.Hits, damageHit{Second: at.Unix(), Source: sourceID, Incoming: incoming, WeaponType: weapon, WeaponStrength: "Misses"})
			}
			continue
		}
		amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
		if err != nil || amount < 0 || math.IsInf(amount, 0) || math.IsNaN(amount) {
			continue
		}
		weapon, strength := damageWeaponDetails(match[3])
		second := at.Unix()
		point := seconds[second]
		if point == nil {
			point = &damagePoint{Second: second}
			seconds[second] = point
		}
		if match[2] == "from" {
			point.Incoming += amount
			point.IncomingHits++
			sourceID, exists := sourceIDs[match[3]]
			if !exists {
				source := incomingDamageSource(match[3])
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
				contribution = &incomingSourcePoint{Second: second, Source: sourceID}
				sourceSeconds[key] = contribution
			}
			contribution.Damage += amount
			contribution.Hits++
			result.Hits = append(result.Hits, damageHit{Second: second, Amount: amount, Source: sourceID, Incoming: true, WeaponType: weapon, WeaponStrength: strength})
		} else {
			point.Outgoing += amount
			point.OutgoingHits++
			result.Hits = append(result.Hits, damageHit{Second: second, Amount: amount, Source: -1, WeaponType: weapon, WeaponStrength: strength})
			target, typeName, recognized := outgoingStructureTarget(match[3])
			structurePending = recognized
			if recognized {
				lastStructure = structureEvent{Second: second, Target: target, Type: typeName}
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

type cachedLog struct {
	Size     int64
	Modified time.Time
	Log      *parsedLog
}

type logIndex struct {
	files map[string]cachedLog
	cache *logCache
}

type logSnapshot struct {
	CachedFiles, ParsedFiles      int
	Logs                          []*parsedLog
	Characters                    []string
	First, Last                   time.Time
	Files, Unassigned, Duplicates int
	SkippedOld, SkippedUndated    int
	Warnings                      []string
	LocationSessions              logLocationHistory
	SourceFiles                   map[string]cachedLog
}

// refresh keeps immutable parsed results for unchanged files. A few workers keep
// the first large import moving without creating a goroutine for every log.
func (index *logIndex) refresh(ctx context.Context, directory string, cutoff time.Time, progress func(int, int)) (*logSnapshot, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("open game logs: %w", err)
	}
	type job struct {
		path string
		info os.FileInfo
	}
	type answer struct {
		path     string
		cached   cachedLog
		err      error
		cacheErr error
		reused   bool
	}
	var jobs []job
	next := make(map[string]cachedLog)
	snapshot := &logSnapshot{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			continue
		}
		// Filter names before stat/open/parse, including cached files. Rebuilding
		// next also releases parsed logs that have aged out or been excluded.
		if !cutoff.IsZero() {
			date, ok := logFilenameDate(entry.Name())
			if !ok {
				snapshot.SkippedUndated++
				continue
			}
			if date.Before(cutoff) {
				snapshot.SkippedOld++
				continue
			}
		}
		path := filepath.Join(directory, entry.Name())
		info, statErr := entry.Info()
		if statErr != nil {
			snapshot.Warnings = append(snapshot.Warnings, entry.Name()+": "+statErr.Error())
			continue
		}
		if cached, ok := index.files[path]; ok && cached.Size == info.Size() && cached.Modified.Equal(info.ModTime()) {
			next[path] = cached
			snapshot.CachedFiles++
		} else {
			jobs = append(jobs, job{path, info})
		}
	}
	snapshot.Files = len(jobs) + len(next)
	work := make(chan job)
	answers := make(chan answer, 4)
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range work {
				parsed, cacheErr := index.cache.load(ctx, task.path, task.info)
				reused := parsed != nil
				var openErr error
				if parsed == nil {
					var file *os.File
					file, openErr = os.Open(task.path)
					if openErr == nil {
						parsed, openErr = parseLog(file)
						if openErr == nil {
							// Do not persist an incomplete read while a live log is growing.
							current, statErr := file.Stat()
							if statErr != nil {
								openErr = statErr
							} else if current.Size() != task.info.Size() || !current.ModTime().Equal(task.info.ModTime()) {
								openErr = fmt.Errorf("log changed while being read; retrying next refresh")
							}
						}
						file.Close()
					}
				}
				cached := cachedLog{task.info.Size(), task.info.ModTime(), parsed}
				if openErr == nil && !reused && ctx.Err() == nil {
					if err := index.cache.store(ctx, task.path, cached); err != nil && cacheErr == nil {
						cacheErr = err
					}
				}
				select {
				case answers <- answer{path: task.path, cached: cached, err: openErr, cacheErr: cacheErr, reused: reused}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, task := range jobs {
			select {
			case work <- task:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(answers) }()
	done := len(next)
	cacheWarned := false
	for answer := range answers {
		if answer.cacheErr != nil && !cacheWarned && ctx.Err() == nil {
			snapshot.Warnings = append(snapshot.Warnings, "SQLite cache unavailable for a log; reading source files: "+answer.cacheErr.Error())
			cacheWarned = true
		}
		if answer.err != nil {
			snapshot.Warnings = append(snapshot.Warnings, filepath.Base(answer.path)+": "+answer.err.Error())
			// Keep the previous successful read if a live file is temporarily locked.
			if previous, ok := index.files[answer.path]; ok {
				next[answer.path] = previous
			}
		} else {
			next[answer.path] = answer.cached
			if answer.reused {
				snapshot.CachedFiles++
			} else {
				snapshot.ParsedFiles++
			}
		}
		done++
		if progress != nil && (done%200 == 0 || done == snapshot.Files) {
			progress(done, snapshot.Files)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index.files = next
	snapshot.SourceFiles = next
	snapshot.populateLogs()
	return snapshot, nil
}

func (snapshot *logSnapshot) populateLogs() {
	characters := make(map[string]bool)
	seen := make(map[[32]byte]bool)
	// Deterministic source ordering also makes duplicate selection stable when
	// the same export is present in a local folder and on a network share.
	paths := make([]string, 0, len(snapshot.SourceFiles))
	for path := range snapshot.SourceFiles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		cached := snapshot.SourceFiles[path]
		log := cached.Log
		if log.Character == "" {
			snapshot.Unassigned++
			continue
		}
		characters[log.Character] = true
		if seen[log.Hash] {
			snapshot.Duplicates++
			continue
		}
		seen[log.Hash] = true
		snapshot.Logs = append(snapshot.Logs, log)
		var first, last time.Time
		if len(log.Points) > 0 {
			first, last = time.Unix(log.Points[0].Second, 0).UTC(), time.Unix(log.Points[len(log.Points)-1].Second, 0).UTC()
		}
		for _, hit := range log.Hits {
			at := time.Unix(hit.Second, 0).UTC()
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if last.IsZero() || at.After(last) {
				last = at
			}
		}
		for _, event := range log.StructureEvents {
			at := time.Unix(event.Second, 0).UTC()
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if last.IsZero() || at.After(last) {
				last = at
			}
		}
		for _, event := range log.EWar {
			at := time.Unix(event.Second, 0).UTC()
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if last.IsZero() || at.After(last) {
				last = at
			}
		}
		for _, event := range log.Logistics {
			at := time.Unix(event.Second, 0).UTC()
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if last.IsZero() || at.After(last) {
				last = at
			}
		}
		for _, event := range log.Mining {
			at := time.Unix(event.Second, 0).UTC()
			if first.IsZero() || at.Before(first) {
				first = at
			}
			if last.IsZero() || at.After(last) {
				last = at
			}
		}
		if !first.IsZero() && (snapshot.First.IsZero() || first.Before(snapshot.First)) {
			snapshot.First = first
		}
		if !last.IsZero() && (snapshot.Last.IsZero() || last.After(snapshot.Last)) {
			snapshot.Last = last
		}
	}
	for character := range characters {
		snapshot.Characters = append(snapshot.Characters, character)
	}
	sort.Strings(snapshot.Characters)
	snapshot.LocationSessions = snapshot.locationSessions()
}
