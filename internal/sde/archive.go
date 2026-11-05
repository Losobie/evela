// Package sde reads the JSONL records in CCP Static Data Export archives.
package sde

import (
	"archive/zip"
	"bufio"
	"fmt"
)

type Archive struct{ *zip.ReadCloser }

func Open(path string) (*Archive, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	return &Archive{reader}, nil
}

// Read streams one member without loading the SDE archive into memory.
// Record bytes belong to the scanner and must not be retained by consume.
func (archive *Archive) Read(name string, consume func([]byte) error) error {
	file, err := archive.ReadCloser.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 8*1024*1024)
	for scanner.Scan() {
		if err := consume(scanner.Bytes()); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return scanner.Err()
}
