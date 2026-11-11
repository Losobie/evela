package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/text/encoding/unicode"
)

const liveLogBase = "Listener: Miner\n" +
	"[ 2026.11.11 02:00:00 ] (mining) You mined 100 units of Loparite\n" +
	"[ 2026.11.11 02:00:01 ] (combat) 200 to Enemy - Laser - Hits\n"
const liveLogBonus = "[ 2026.11.11 02:00:02 ] (mining) Critical mining success! You mined an additional 156 units of Loparite"

func encodeLiveLog(t *testing.T, encoding, text string) []byte {
	t.Helper()
	if encoding == "utf8" {
		return []byte(text)
	}
	order := unicode.LittleEndian
	if encoding == "utf16be" {
		order = unicode.BigEndian
	}
	data, err := unicode.UTF16(order, unicode.UseBOM).NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func appendLiveLog(t *testing.T, path string, data []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
}

func TestLiveLogPartialLineCompletionAndCache(t *testing.T) {
	for _, encoding := range []string{"utf8", "utf16le", "utf16be"} {
		t.Run(encoding, func(t *testing.T) {
			folder := t.TempDir()
			path := filepath.Join(folder, "20261111_live.txt")
			partial := encodeLiveLog(t, encoding, liveLogBase+liveLogBonus)
			if err := os.WriteFile(path, partial, 0600); err != nil {
				t.Fatal(err)
			}
			cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
			index := &logIndex{cache: cache}
			first := cacheScan(t, index, folder, time.Time{})
			if len(first.Warnings) != 0 || len(first.Logs) != 1 || len(first.Logs[0].Events) != 2 || len(first.Logs[0].Mining) != 1 || len(first.Logs[0].Hits) != 1 {
				t.Fatalf("complete records lost or unfinished bonus published: %+v", first)
			}
			var count int
			if err := cache.db.QueryRow("SELECT count(*) FROM log_files").Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial snapshot persisted: %d, %v", count, err)
			}
			retry := cacheScan(t, index, folder, time.Time{})
			if retry.ParsedFiles != 1 || len(retry.Logs[0].Mining) != 1 {
				t.Fatal("partial file was cached in memory or duplicated")
			}
			finished := encodeLiveLog(t, encoding, liveLogBase+liveLogBonus+"\n")
			appendLiveLog(t, path, finished[len(partial):])
			second := cacheScan(t, index, folder, time.Time{})
			if len(second.Warnings) != 0 || len(second.Logs[0].Events) != 3 || len(second.Logs[0].Mining) != 2 || !second.Logs[0].Mining[1].Critical || second.Logs[0].Mining[1].Amount != 156 {
				t.Fatal("completed critical bonus missing or duplicated")
			}
			if len(first.Logs[0].Events) != 2 {
				t.Fatal("refresh modified a published snapshot")
			}
			restarted := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
			if restarted.CachedFiles != 1 || len(restarted.Logs[0].Mining) != 2 {
				t.Fatal("completed file did not enter the persistent cache")
			}
		})
	}
}

type appendingLogFile struct {
	*os.File
	afterRead func()
}

func (file *appendingLogFile) ReadAt(data []byte, offset int64) (int, error) {
	n, err := file.File.ReadAt(data, offset)
	if file.afterRead != nil {
		action := file.afterRead
		file.afterRead = nil
		action()
	}
	return n, err
}

func TestLiveLogGrowthDuringRead(t *testing.T) {
	path := writeCacheLog(t, t.TempDir(), "20261111_live.txt", liveLogBase)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	live := &appendingLogFile{File: file, afterRead: func() { appendLiveLog(t, path, []byte(liveLogBonus+"\n")) }}
	first, err := readLogSnapshot(live)
	if err != nil || first.Size >= 0 || len(first.Log.Mining) != 1 || len(first.Log.Hits) != 1 {
		t.Fatalf("growth rejected complete records or made snapshot cacheable: %+v, %v", first, err)
	}
	second, err := readLogSnapshot(file)
	if err != nil || second.Size < 0 || len(second.Log.Mining) != 2 {
		t.Fatalf("next read did not include appended bonus: %+v, %v", second, err)
	}
}

func TestCompleteLogPrefixUTF16AlignmentAndLongTail(t *testing.T) {
	for _, encoding := range []string{"utf16le", "utf16be"} {
		data := encodeLiveLog(t, encoding, liveLogBase+liveLogBonus)
		path := filepath.Join(t.TempDir(), "live.txt")
		if err := os.WriteFile(path, data[:len(data)-1], 0600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := readLogSnapshot(file)
		file.Close()
		if err != nil || len(entry.Log.Events) != 2 {
			t.Fatalf("split UTF-16 code unit: %+v, %v", entry, err)
		}
	}
	// A long unfinished line can cross multiple backwards-search blocks.
	path := writeCacheLog(t, t.TempDir(), "live.txt", liveLogBase)
	appendLiveLog(t, path, make([]byte, 150000))
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	end, err := completeLogPrefix(file, int64(len(liveLogBase)+150000))
	if err != nil || end != int64(len(liveLogBase)) {
		t.Fatalf("incorrect long-tail boundary: %d, %v", end, err)
	}
	if _, err := completeLogPrefix(file, int64(len(liveLogBase)+200000)); err != io.EOF {
		t.Fatalf("short read not reported: %v", err)
	}
}
