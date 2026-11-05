package parser

import "strings"

func nameKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// HullClass returns the exact catalog class, or an empty string if unknown.
func HullClass(name string) string { return loadHullClasses()[nameKey(name)] }

// NPCGroup returns the catalog group label, or an empty string if unknown.
func NPCGroup(id int) string { return loadNPCCatalog().Groups[id] }

// LookupNPC matches an exact display name, ignoring case and outer whitespace.
func LookupNPC(name string) (NPCIdentity, bool) {
	identity, ok := loadNPCCatalog().names[nameKey(name)]
	return identity, ok
}
