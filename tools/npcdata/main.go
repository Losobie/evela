// Command npcdata extracts a compact NPC lookup from CCP's official JSONL SDE.
// Usage: go run ./tools/npcdata -sde .cache/eve-sde-<build>-jsonl.zip
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"evela/internal/sde"
)

type npcType struct {
	ID      int      `json:"id"`
	GroupID int      `json:"group"`
	Names   []string `json:"names"`
}

func main() {
	archive := flag.String("sde", "", "official CCP JSONL SDE zip")
	output := flag.String("out", "parser/npc_data.json.gz", "generated lookup")
	flag.Parse()
	if err := generate(*archive, *output); err != nil {
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
		Build   int            `json:"build"`
		Release string         `json:"release"`
		Groups  map[int]string `json:"groups"`
		Types   []npcType      `json:"types"`
	}{Groups: map[int]string{}}
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
	if err := archive.Read("groups.jsonl", func(line []byte) error {
		var record struct {
			ID       int               `json:"_key"`
			Category int               `json:"categoryID"`
			Name     map[string]string `json:"name"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		// Entity (11) contains NPC ships, sentries and NPC structures. Do not
		// require published=true: most combat NPCs are unpublished SDE types.
		if record.Category == 11 && record.Name["en"] != "" {
			data.Groups[record.ID] = record.Name["en"]
		}
		return nil
	}); err != nil {
		return err
	}
	if err := archive.Read("types.jsonl", func(line []byte) error {
		var record struct {
			ID    int               `json:"_key"`
			Group int               `json:"groupID"`
			Name  map[string]string `json:"name"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			return err
		}
		if data.Groups[record.Group] == "" {
			return nil
		}
		item := npcType{ID: record.ID, GroupID: record.Group}
		seen := map[string]bool{}
		for _, name := range record.Name {
			if name != "" && !seen[name] {
				item.Names = append(item.Names, name)
				seen[name] = true
			}
		}
		sort.Strings(item.Names)
		data.Types = append(data.Types, item)
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(data.Types, func(i, j int) bool { return data.Types[i].ID < data.Types[j].ID })
	if data.Build == 0 || len(data.Groups) == 0 || len(data.Types) == 0 {
		return fmt.Errorf("SDE is missing build, NPC groups or NPC types")
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	writer := gzip.NewWriter(file)
	err = json.NewEncoder(writer).Encode(data)
	err = firstError(err, writer.Close(), file.Close())
	if err == nil {
		fmt.Printf("SDE %d (%s): %d NPC types, %d groups → %s\n", data.Build, data.Release, len(data.Types), len(data.Groups), output)
	}
	return err
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
