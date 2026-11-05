package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// Ship, starbase and structure classes from CCP's official SDE. EVE Online and
// its game data are copyright CCP hf. See README for the regeneration command.
//
//go:embed hull_data.json
var hullData []byte

var loadHullClasses = sync.OnceValue(func() map[string]string {
	var catalog struct{ Names map[string]string }
	if err := json.Unmarshal(hullData, &catalog); err != nil {
		panic(fmt.Sprintf("embedded hull classes: %v", err))
	}
	classes := make(map[string]string, len(catalog.Names))
	for name, class := range catalog.Names {
		classes[characterKey(name)] = class
	}
	return classes
})
