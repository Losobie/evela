package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testLogCache(t *testing.T, path string, version int) *logCache {
	t.Helper()
	cache, err := openLogCache(path, version)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cache.close() })
	return cache
}
func cacheScan(t *testing.T, index *logIndex, folder string, cutoff time.Time) *logSnapshot {
	t.Helper()
	snapshot, err := index.refresh(context.Background(), folder, cutoff, nil)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func writeCacheLog(t *testing.T, folder, name, content string) string {
	t.Helper()
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSQLiteCachePersistsAcrossRestartAndReprocessesChangedFiles(t *testing.T) {
	root, folder := t.TempDir(), t.TempDir()
	dbPath := filepath.Join(root, "cache space # Unicode é", "logs.sqlite")
	path := writeCacheLog(t, folder, "20261017_a.txt", sampleLog)
	cache := testLogCache(t, dbPath, logParserVersion)
	first := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if first.ParsedFiles != 1 || first.CachedFiles != 0 {
		t.Fatalf("first import stats: %+v", first)
	}
	cache.close()
	cache = testLogCache(t, dbPath, logParserVersion)
	second := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if second.CachedFiles != 1 || second.ParsedFiles != 0 || !reflect.DeepEqual(first.Logs, second.Logs) {
		t.Fatal("restart reparsed unchanged input or lost parsed fields")
	}
	// Same-length edits still invalidate when the modification timestamp changes.
	content := strings.ReplaceAll(sampleLog, "Test Pilot", "Next Pilot")
	writeCacheLog(t, folder, filepath.Base(path), content)
	stamp := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	third := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if third.ParsedFiles != 1 || third.Logs[0].Character != "Next Pilot" {
		t.Fatal("same-size changed file was not reprocessed")
	}
	writeCacheLog(t, folder, "20261017_b.txt", strings.ReplaceAll(sampleLog, "Test Pilot", "Added Pilot"))
	fourth := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if fourth.Files != 2 || fourth.ParsedFiles != 1 || fourth.CachedFiles != 1 {
		t.Fatal("new file from merged folder was not discovered independently")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	fifth := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if fifth.Files != 1 || len(fifth.Logs) != 1 || fifth.Logs[0].Character != "Added Pilot" {
		t.Fatal("deleted source survived solely through database cache")
	}
	// Opening with a new parser version must ignore every old result.
	next := testLogCache(t, dbPath, logParserVersion+1)
	reprocessed := cacheScan(t, &logIndex{cache: next}, folder, time.Time{})
	if reprocessed.ParsedFiles != 1 || reprocessed.CachedFiles != 0 {
		t.Fatal("parser version bump did not reprocess unchanged source")
	}
	reused := cacheScan(t, &logIndex{cache: next}, folder, time.Time{})
	if reused.CachedFiles != 1 || reused.ParsedFiles != 0 {
		t.Fatal("new parser results were not persisted")
	}
}

func TestSQLiteCacheRoundTripsAllParsedEventTypes(t *testing.T) {
	folder := t.TempDir()
	content := sampleLog + logisticsTestLog + miningTestLog +
		"[ 2026.10.20 02:00:01 ] (notify) Jumping from Jita to Amarr.\n" +
		"[ 2026.10.20 02:00:02 ] (combat) 912 to Grijawlva[SNOOO](Phoenix Navy Issue) - 425mm Railgun II - Penetrates\n" +
		"[ 2026.10.20 02:00:03 ] (combat) 120 remote shield boosted to Grijawlva[SNOOO](Phoenix Navy Issue)\n" +
		"[ 2026.10.20 02:00:04 ] (combat) 472 remote shield boosted to Customs Office (R3-K7K IX) [Way So Mad] - Large Remote Shield Booster II\n" +
		"[ 2026.10.20 02:00:05 ] " + bracketedLogisticsExample + "\n" +
		"[ 2026.10.20 02:00:06 ] " + unalliedLogisticsExample + "\n" +
		"[ 2026.10.20 02:00:07 ] (combat) 466 to Blood Raiders Stronghold - Ogre I - Penetrates\n" +
		"[ 2026.10.20 02:00:08 ] (combat) 376 to Guristas Pirates Stronghold - Ogre I - Hits\n"
	writeCacheLog(t, folder, "20261020_all.txt", content)
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	first := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	second := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if second.CachedFiles != 1 || !reflect.DeepEqual(first.SourceFiles, second.SourceFiles) {
		t.Fatal("persistent serialization lost parsed data")
	}
	for _, log := range second.Logs {
		foundHit, foundLogistics, foundCustomsOffice := false, false, false
		foundBracketedLogistics := false
		foundUnalliedLogistics := false
		strongholds := map[string]bool{}
		for _, hit := range log.Hits {
			foundHit = foundHit || hit.Target == "Grijawlva" && hit.TargetType == "Phoenix Navy Issue"
			if (hit.Target == "Blood Raiders Stronghold" || hit.Target == "Guristas Pirates Stronghold") && hit.TargetType == "♦ Stronghold" {
				strongholds[hit.Target] = true
			}
		}
		for _, event := range log.Logistics {
			foundLogistics = foundLogistics || event.Target == "Grijawlva" && event.TargetType == "Phoenix Navy Issue"
			foundCustomsOffice = foundCustomsOffice || event.Target == "Customs Office" && event.TargetType == "Customs Office" && event.TargetLocation == "R3-K7K IX" && event.TargetCorporation == "Way So Mad" && event.TargetCorporationTicker == ""
			foundBracketedLogistics = foundBracketedLogistics || event.Target == "Clento Loso" && event.TargetType == "Basilisk" && event.TargetCorporationTicker == ".AE.." && event.TargetAllianceTicker == "THIRD"
			foundUnalliedLogistics = foundUnalliedLogistics || event.Target == "Alpha Arterius" && event.TargetType == "Stork" && event.TargetCorporationTicker == "53C0N" && event.TargetAllianceTicker == "" && event.Amount == 0
		}
		if !foundHit || !foundLogistics || !foundCustomsOffice || !foundBracketedLogistics || !foundUnalliedLogistics || len(strongholds) != 2 {
			t.Fatal("persistent cache lost target fields")
		}
		if len(log.Hits) == 0 || len(log.Mining) == 0 || len(log.Logistics) == 0 || len(log.EWar) == 0 || len(log.LocationEvents) == 0 {
			t.Fatal("fixture does not exercise all event families")
		}
	}
}

func TestSQLiteCacheCorruptPayloadFallsBackAndRepairs(t *testing.T) {
	folder := t.TempDir()
	writeCacheLog(t, folder, "20261017_a.txt", sampleLog)
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	first := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if _, err := cache.db.Exec("UPDATE parsed_logs SET payload=?", []byte("corrupt")); err != nil {
		t.Fatal(err)
	}
	repaired := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if repaired.ParsedFiles != 1 || len(repaired.Warnings) != 1 || !reflect.DeepEqual(first.Logs, repaired.Logs) {
		t.Fatal("bad payload did not fall back safely")
	}
	reused := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if reused.CachedFiles != 1 || len(reused.Warnings) != 0 || cache.disabled.Load() {
		t.Fatal("bad payload was not repaired")
	}
	cache.close()
	fallback := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if fallback.ParsedFiles != 1 || len(fallback.Warnings) != 1 || !reflect.DeepEqual(first.Logs, fallback.Logs) {
		t.Fatal("cache failure prevented source import")
	}
}

func TestSQLiteCacheAtomicWritesAndLiveFileGrowth(t *testing.T) {
	folder := t.TempDir()
	path := writeCacheLog(t, folder, "20261017_a.txt", sampleLog)
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	for index := 0; index < 3; index++ {
		content := sampleLog + fmt.Sprintf("[ 2026.10.17 02:01:03 ] (combat) %d to Enemy - Laser - Hits\n", index+1)
		writeCacheLog(t, folder, filepath.Base(path), content)
		cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	}
	var count int
	if err := cache.db.QueryRow("SELECT count(*) FROM parsed_logs").Scan(&count); err != nil || count != 1 {
		t.Fatalf("growing live files leaked old cached versions: count=%d err=%v", count, err)
	}
	if _, err := cache.db.Exec(`CREATE TRIGGER fail_manifest BEFORE INSERT ON log_files BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	writeCacheLog(t, folder, "20261017_b.txt", strings.ReplaceAll(sampleLog, "Test Pilot", "Other Pilot"))
	failed := cacheScan(t, &logIndex{cache: cache}, folder, time.Time{})
	if failed.Files != 2 || failed.ParsedFiles != 1 || len(failed.Warnings) != 1 || !cache.disabled.Load() {
		t.Fatal("failed cache write discarded successfully parsed log")
	}
	if err := cache.db.QueryRow("SELECT count(*) FROM parsed_logs").Scan(&count); err != nil || count != 1 {
		t.Fatal("failed manifest write left a partially committed payload")
	}
}

func TestSQLiteCacheMultiFolderDeduplicationAndCutoff(t *testing.T) {
	folderA, folderB := t.TempDir(), t.TempDir()
	writeCacheLog(t, folderA, "20261017_a.txt", sampleLog)
	writeCacheLog(t, folderB, "20261017_copy.txt", sampleLog)
	writeCacheLog(t, folderB, "20251017_old.txt", strings.ReplaceAll(sampleLog, "Test Pilot", "Old Pilot"))
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	index := &multiLogIndex{cache: cache}
	defer index.close()
	first, err := index.refresh(context.Background(), []string{folderA, folderB}, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != 3 || first.Duplicates != 1 || first.ParsedFiles != 3 {
		t.Fatal("initial multi-folder import failed")
	}
	var count int
	if err = cache.db.QueryRow("SELECT count(*) FROM parsed_logs").Scan(&count); err != nil || count != 2 {
		t.Fatal("identical files stored redundant parsed payloads")
	}
	restart := &multiLogIndex{cache: cache}
	defer restart.close()
	second, err := restart.refresh(context.Background(), []string{folderA, folderB}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.CachedFiles != 2 || second.ParsedFiles != 0 || second.Duplicates != 1 || second.SkippedOld != 1 || len(second.Logs) != 1 {
		t.Fatal("persistent reuse changed duplicate or loading-limit behavior")
	}
}

func TestSQLiteCacheRejectsNewerSchemaAndInvalidDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs.sqlite")
	cache := testLogCache(t, path, logParserVersion)
	if _, err := cache.db.Exec(fmt.Sprintf("PRAGMA user_version=%d", logCacheSchemaVersion+1)); err != nil {
		t.Fatal(err)
	}
	cache.close()
	if other, err := openLogCache(path, logParserVersion); err == nil {
		other.close()
		t.Fatal("newer cache schema was accepted")
	}
	// A newer database is never downgraded or erased.
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != logCacheSchemaVersion+1 {
		t.Fatal("newer schema was changed")
	}
	badPath := filepath.Join(t.TempDir(), "logs.sqlite")
	original := []byte("not a SQLite database")
	if err = os.WriteFile(badPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := openLogCache(badPath, logParserVersion); err == nil {
		other.close()
		t.Fatal("invalid cache accepted")
	}
	after, err := os.ReadFile(badPath)
	if err != nil || string(after) != string(original) {
		t.Fatal("invalid database was erased")
	}
}

func TestSQLiteCacheCancellationDoesNotPublishPartialEntry(t *testing.T) {
	folder := t.TempDir()
	path := writeCacheLog(t, folder, "20261017_a.txt", sampleLog)
	cache := testLogCache(t, filepath.Join(t.TempDir(), "logs.sqlite"), logParserVersion)
	log, err := parseLog(strings.NewReader(sampleLog))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = cache.store(ctx, path, cachedLog{Size: info.Size(), Modified: info.ModTime(), Log: log}); err == nil {
		t.Fatal("cancelled cache write succeeded")
	}
	if cache.disabled.Load() {
		t.Fatal("cancelled scan disabled the shared cache")
	}
	var count int
	if err = cache.db.QueryRow("SELECT count(*) FROM log_files").Scan(&count); err != nil || count != 0 {
		t.Fatal("cancelled write published a manifest")
	}
	if err = cache.db.QueryRow("SELECT count(*) FROM parsed_logs").Scan(&count); err != nil || count != 0 {
		t.Fatal("cancelled write published a payload")
	}
}

func BenchmarkParsedLogPersistence(b *testing.B) {
	var content strings.Builder
	content.WriteString("Listener: Pilot\n")
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&content, "[ %s ] (combat) %d to Enemy - Heavy Beam Laser II - Penetrates\n", time.Unix(1792000000+int64(i), 0).UTC().Format(logTimeLayout), i%1000+1)
	}
	source := content.String()
	folder := b.TempDir()
	path := filepath.Join(folder, "20261017_a.txt")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		b.Fatal(err)
	}
	cache, err := openLogCache(filepath.Join(b.TempDir(), "logs.sqlite"), logParserVersion)
	if err != nil {
		b.Fatal(err)
	}
	defer cache.close()
	log, err := parseLog(strings.NewReader(source))
	if err != nil {
		b.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	if err = cache.store(context.Background(), path, cachedLog{Size: info.Size(), Modified: info.ModTime(), Log: log}); err != nil {
		b.Fatal(err)
	}
	b.Run("ParseSource", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := parseLog(strings.NewReader(source)); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("LoadSQLite", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if log, err := cache.load(context.Background(), path, info); err != nil || log == nil {
				b.Fatalf("load: %v", err)
			}
		}
	})
}
