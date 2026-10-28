package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Derived from CCP's official SDE, category 25 (Asteroid).
// EVE Online and its game data are copyright CCP hf. See README for regeneration.
//
//go:embed ore_data.json
var oreData []byte

var loadOreTypes = sync.OnceValue(func() map[string]string {
	var catalog struct {
		Names map[string]string `json:"names"`
	}
	if err := json.Unmarshal(oreData, &catalog); err != nil {
		panic(fmt.Sprintf("embedded ore data: %v", err))
	}
	names := make(map[string]string, len(catalog.Names))
	for name, group := range catalog.Names {
		names[strings.ToLower(name)] = group
	}
	return names
})

func oreType(name string) string {
	names := loadOreTypes()
	key := strings.ToLower(strings.TrimSpace(name))
	// Exact names preserve every SDE grade. Historical grades absent from the
	// current SDE (e.g. Prismatic Gneiss) may still identify a known ore suffix.
	for {
		if group := names[key]; group != "" {
			return group
		}
		index := strings.IndexByte(key, ' ')
		if index < 0 {
			return "Unknown ore type"
		}
		key = key[index+1:]
	}
}
