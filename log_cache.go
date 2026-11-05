package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Increment whenever parsing, extracted fields, source classification, embedded
// catalogs, or the serialized parsedLog format changes. Old results are never
// used by a new parser version, even when source files have not changed.
const logParserVersion = 5
const logCacheSchemaVersion = 1

type logCache struct {
	disabled      atomic.Bool
	db            *sql.DB
	parserVersion int
}

func defaultLogCachePath() (string, error) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "Evela", "logs.sqlite"), nil
}

func openLogCache(path string, parserVersion int) (*logCache, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	query := uri.Query()
	query.Set("_busy_timeout", "1000")
	query.Set("_journal_mode", "WAL")
	query.Set("_synchronous", "NORMAL")
	query.Set("_foreign_keys", "on")
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite3", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	cache := &logCache{db: db, parserVersion: parserVersion}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = cache.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return cache, nil
}

func (cache *logCache) initialize(ctx context.Context) error {
	var version int
	if err := cache.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > logCacheSchemaVersion {
		return fmt.Errorf("log cache schema %d is newer than supported schema %d", version, logCacheSchemaVersion)
	}
	tx, err := cache.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS parsed_logs (
   hash BLOB NOT NULL,
   parser_version INTEGER NOT NULL,
   payload BLOB NOT NULL,
   payload_hash BLOB NOT NULL,
   PRIMARY KEY (hash, parser_version)
  ) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS log_files (
   path TEXT NOT NULL,
   parser_version INTEGER NOT NULL,
   size INTEGER NOT NULL,
   modified_ns INTEGER NOT NULL,
   hash BLOB NOT NULL,
   PRIMARY KEY (path, parser_version),
   FOREIGN KEY (hash, parser_version) REFERENCES parsed_logs(hash, parser_version)
  ) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS log_files_by_hash ON log_files(hash,parser_version)`,
		fmt.Sprintf("PRAGMA user_version = %d", logCacheSchemaVersion),
	} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (cache *logCache) close() error {
	if cache == nil {
		return nil
	}
	return cache.db.Close()
}

func (cache *logCache) load(ctx context.Context, path string, info os.FileInfo) (*parsedLog, error) {
	if cache == nil || cache.disabled.Load() {
		return nil, nil
	}
	var payload, checksum, hash []byte
	err := cache.db.QueryRowContext(ctx, `SELECT p.payload, p.payload_hash, p.hash
  FROM log_files f JOIN parsed_logs p ON p.hash=f.hash AND p.parser_version=f.parser_version
  WHERE f.path=? AND f.parser_version=? AND f.size=? AND f.modified_ns=?`,
		logFolderKey(path), cache.parserVersion, info.Size(), info.ModTime().UnixNano()).Scan(&payload, &checksum, &hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		if ctx.Err() == nil {
			cache.disabled.Store(true)
		}
		return nil, err
	}
	actual := sha256.Sum256(payload)
	if !bytes.Equal(checksum, actual[:]) {
		return nil, fmt.Errorf("cached payload checksum mismatch")
	}
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var log parsedLog
	if err = gob.NewDecoder(reader).Decode(&log); err != nil {
		return nil, err
	}
	if !bytes.Equal(log.Hash[:], hash) {
		return nil, fmt.Errorf("cached source hash mismatch")
	}
	return &log, nil
}

func (cache *logCache) store(ctx context.Context, path string, entry cachedLog) (err error) {
	if cache == nil || cache.disabled.Load() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Avoid retrying an unavailable database for every file in a large import.
	// Cancellation is per scan and must not disable the cache for other folders.
	defer func() {
		if err != nil && ctx.Err() == nil {
			cache.disabled.Store(true)
		}
	}()
	var payload bytes.Buffer
	writer, err := gzip.NewWriterLevel(&payload, gzip.BestSpeed)
	if err != nil {
		return err
	}
	if err = gob.NewEncoder(writer).Encode(entry.Log); err != nil {
		writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	checksum := sha256.Sum256(payload.Bytes())
	tx, err := cache.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous []byte
	err = tx.QueryRowContext(ctx, "SELECT hash FROM log_files WHERE path=? AND parser_version=?", logFolderKey(path), cache.parserVersion).Scan(&previous)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO parsed_logs(hash,parser_version,payload,payload_hash)
  VALUES(?,?,?,?) ON CONFLICT(hash,parser_version) DO UPDATE SET payload=excluded.payload,payload_hash=excluded.payload_hash`,
		entry.Log.Hash[:], cache.parserVersion, payload.Bytes(), checksum[:]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO log_files(path,parser_version,size,modified_ns,hash)
  VALUES(?,?,?,?,?) ON CONFLICT(path,parser_version) DO UPDATE SET size=excluded.size,modified_ns=excluded.modified_ns,hash=excluded.hash`,
		logFolderKey(path), cache.parserVersion, entry.Size, entry.Modified.UnixNano(), entry.Log.Hash[:]); err != nil {
		return err
	}
	// Live files acquire a new content hash on each refresh. Retain an old
	// payload only while another imported path still refers to that content.
	if len(previous) > 0 && !bytes.Equal(previous, entry.Log.Hash[:]) {
		if _, err = tx.ExecContext(ctx, `DELETE FROM parsed_logs WHERE hash=? AND parser_version=?
          AND NOT EXISTS (SELECT 1 FROM log_files WHERE hash=? AND parser_version=?)`, previous, cache.parserVersion, previous, cache.parserVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}
