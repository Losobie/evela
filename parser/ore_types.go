package parser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Derived from CCP's official SDE, category 25 (Asteroid).
// EVE Online and its game data are copyright CCP hf. See docs/DEVELOPMENT.md for regeneration.
//
//go:embed ore_data.json
var oreData []byte

type oreCatalog struct {
	Names   map[string]string  `json:"names"`
	Volumes map[string]float64 `json:"volumes"`
}

var loadOreCatalog = sync.OnceValue(func() oreCatalog {
	var catalog oreCatalog
	if err := json.Unmarshal(oreData, &catalog); err != nil {
		panic(fmt.Sprintf("embedded ore data: %v", err))
	}
	names := make(map[string]string, len(catalog.Names))
	for name, group := range catalog.Names {
		names[strings.ToLower(name)] = group
	}
	volumes := make(map[string]float64, len(catalog.Volumes))
	for name, volume := range catalog.Volumes {
		volumes[strings.ToLower(name)] = volume
	}
	return oreCatalog{Names: names, Volumes: volumes}
})

func OreType(name string) string {
	names := loadOreCatalog().Names
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

// OreVolume returns cubic metres per unit. Historical ore grades can use a
// known base ore, but missing compressed types must never use raw ore volume.
func OreVolume(name string) (float64, bool) {
	volumes := loadOreCatalog().Volumes
	key := strings.ToLower(strings.TrimSpace(name))
	compressed := strings.HasPrefix(key, "compressed ")
	for {
		if volume, ok := volumes[key]; ok {
			return volume, true
		}
		index := strings.IndexByte(key, ' ')
		if compressed || index < 0 {
			return 0, false
		}
		key = key[index+1:]
	}
}
