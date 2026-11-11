package main

import (
	"fmt"
	"io"
	"os"
)

type logSnapshotFile interface {
	io.ReaderAt
	Stat() (os.FileInfo, error)
}

// Read a fixed prefix rather than chasing EOF as EVE appends new messages.
// Only newline-terminated records are safe to publish from a live file.
func readLogSnapshot(file logSnapshotFile) (cachedLog, error) {
	before, err := file.Stat()
	if err != nil {
		return cachedLog{}, err
	}
	end, err := completeLogPrefix(file, before.Size())
	if err != nil {
		return cachedLog{}, err
	}
	parsed, err := parseLog(io.NewSectionReader(file, 0, end))
	if err != nil {
		return cachedLog{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return cachedLog{}, err
	}
	if after.Size() < before.Size() || after.Size() == before.Size() && !after.ModTime().Equal(before.ModTime()) {
		return cachedLog{}, fmt.Errorf("log was truncated or rewritten while being read; retrying next refresh")
	}
	entry := cachedLog{Size: before.Size(), Modified: before.ModTime(), Log: parsed}
	if end != before.Size() || after.Size() != before.Size() {
		// Publish the complete records now, but retry next refresh and never
		// store this prefix as a complete file in the persistent cache.
		entry.Size = -1
	}
	return entry, nil
}

// Find a complete line boundary without buffering the whole file. UTF-16
// newlines must be matched as aligned code units, including the BOM offset.
func completeLogPrefix(file io.ReaderAt, size int64) (int64, error) {
	if size == 0 {
		return 0, nil
	}
	var bom [2]byte
	if _, err := file.ReadAt(bom[:min(size, 2)], 0); err != nil {
		return 0, err
	}
	width := int64(1)
	newline := []byte{'\n'}
	if bom == [2]byte{0xff, 0xfe} {
		width, newline = 2, []byte{'\n', 0}
	} else if bom == [2]byte{0xfe, 0xff} {
		width, newline = 2, []byte{0, '\n'}
	}
	buffer := make([]byte, 64*1024)
	for end := size - size%width; end > 0; {
		start := max(int64(0), end-int64(len(buffer)))
		chunk := buffer[:end-start]
		if _, err := file.ReadAt(chunk, start); err != nil {
			return 0, err
		}
		for offset := int64(len(chunk)) - width; offset >= 0; offset -= width {
			if chunk[offset] == newline[0] && (width == 1 || chunk[offset+1] == newline[1]) {
				return start + offset + width, nil
			}
		}
		end = start
	}
	return 0, nil
}
