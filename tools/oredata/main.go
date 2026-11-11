// Command oredata extracts ore names, groups and unit volumes from CCP's official JSONL SDE.
// Usage: go run ./tools/oredata -sde .cache/eve-sde-<build>-jsonl.zip
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"evela/internal/sde"
)

func main() {
	path := flag.String("sde", "", "official CCP JSONL SDE zip")
	output := flag.String("out", "parser/ore_data.json", "generated lookup")
	flag.Parse()
	if err := generate(*path, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(path, output string) error {
	archive, err := sde.Open(path)
	if err != nil {
		return err
	}
	defer archive.Close()

	data := struct {
		Build   int                `json:"build"`
		Release string             `json:"release"`
		Names   map[string]string  `json:"names"`
		Volumes map[string]float64 `json:"volumes"`
	}{Names: map[string]string{}, Volumes: map[string]float64{}}
	if err := archive.Read("_sde.jsonl", func(line []byte) error {
		var record struct {
			Key     string `json:"_key"`
			Build   int    `json:"buildNumber"`
			Release string `json:"releaseDate"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if record.Key == "sde" {
			data.Build, data.Release = record.Build, record.Release
		}
		return nil
	}); err != nil {
		return err
	}
	groups := map[int]string{}
	if err := archive.Read("groups.jsonl", func(line []byte) error {
		var record struct {
			ID       int               `json:"_key"`
			Category int               `json:"categoryID"`
			Name     map[string]string `json:"name"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if record.Category == 25 && record.Name["en"] != "" {
			// Present moon resource groups using the terminology in the filter.
			groups[record.ID] = strings.ReplaceAll(record.Name["en"], "Moon Asteroids", "Moon Ore")
		}
		return nil
	}); err != nil {
		return err
	}
	if err := archive.Read("types.jsonl", func(line []byte) error {
		var record struct {
			Group  int               `json:"groupID"`
			Name   map[string]string `json:"name"`
			Volume float64           `json:"volume"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if group, name := groups[record.Group], record.Name["en"]; group != "" && name != "" {
			data.Names[name] = group
			if record.Volume > 0 {
				data.Volumes[name] = record.Volume
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if data.Build == 0 || len(data.Names) == 0 {
		return fmt.Errorf("SDE is missing build or ore types")
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, append(encoded, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("SDE %d (%s): %d ore names → %s\n", data.Build, data.Release, len(data.Names), output)
	return nil
}
