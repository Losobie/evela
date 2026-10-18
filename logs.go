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

type parsedLog struct {
	Character       string
	Points          []damagePoint
	StructureEvents []structureEvent
	EntrySeconds    []int64
	Hash            [32]byte
}

// parseLog accepts EVE's UTF-8 and BOM-marked UTF-16 exports. Rates use numeric
// damage messages; structure invulnerability notifications are recorded separately.
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
		channelEnd := strings.IndexByte(message, ')')
		if !strings.HasPrefix(message, "(") || channelEnd < 0 {
			continue
		}
		combat := message[:channelEnd+1] == "(combat)"
		if !combat && !strings.Contains(message, "invulnerable") {
			continue
		}
		message = strings.TrimSpace(html.UnescapeString(markup.ReplaceAllString(message[channelEnd+1:], "")))
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
			continue
		}
		amount, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
		if err != nil || amount < 0 || math.IsInf(amount, 0) || math.IsNaN(amount) {
			continue
		}
		second := at.Unix()
		point := seconds[second]
		if point == nil {
			point = &damagePoint{Second: second}
			seconds[second] = point
		}
		if match[2] == "from" {
			point.Incoming += amount
			point.IncomingHits++
		} else {
			point.Outgoing += amount
			point.OutgoingHits++
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
	for _, point := range seconds {
		result.Points = append(result.Points, *point)
	}
	sort.Slice(result.Points, func(i, j int) bool { return result.Points[i].Second < result.Points[j].Second })
	sort.Slice(result.StructureEvents, func(i, j int) bool { return result.StructureEvents[i].Second < result.StructureEvents[j].Second })
	return result, nil
}

type cachedLog struct {
	Size     int64
	Modified time.Time
	Log      *parsedLog
}

type logIndex struct{ files map[string]cachedLog }

type logSnapshot struct {
	Logs                          []*parsedLog
	Characters                    []string
	First, Last                   time.Time
	Files, Unassigned, Duplicates int
	SkippedOld, SkippedUndated    int
	Warnings                      []string
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
		path   string
		cached cachedLog
		err    error
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
				file, openErr := os.Open(task.path)
				var parsed *parsedLog
				if openErr == nil {
					parsed, openErr = parseLog(file)
					file.Close()
				}
				select {
				case answers <- answer{task.path, cachedLog{task.info.Size(), task.info.ModTime(), parsed}, openErr}:
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
	for answer := range answers {
		if answer.err != nil {
			snapshot.Warnings = append(snapshot.Warnings, filepath.Base(answer.path)+": "+answer.err.Error())
			// Keep the previous successful read if a live file is temporarily locked.
			if previous, ok := index.files[answer.path]; ok {
				next[answer.path] = previous
			}
		} else {
			next[answer.path] = answer.cached
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
	characters := make(map[string]bool)
	seen := make(map[[32]byte]bool)
	for _, cached := range next {
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
		for _, event := range log.StructureEvents {
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
	return snapshot, nil
}
