package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

const characterGroupsPreference = "characters.groups.v1"
const characterGroupsMode = "Character groups"
const ungroupedCharacters = "Ungrouped characters"

type characterGroup struct {
	ID      string
	Name    string
	Members []string
}

func sortedCharacterNames(names map[string]string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name)
	}
	sort.Slice(result, func(i, j int) bool { return compareTotalsText(result[i], result[j]) < 0 })
	return result
}

func cleanGroupMembers(members []string) []string {
	names := map[string]string{}
	for _, name := range members {
		if name = strings.TrimSpace(name); name != "" {
			names[characterKey(name)] = name
		}
	}
	return sortedCharacterNames(names)
}

func (group characterGroup) contains(name string) bool {
	for _, member := range group.Members {
		if characterKey(member) == characterKey(name) {
			return true
		}
	}
	return false
}

func (ui *analyzerUI) loadCharacterGroups() {
	var saved []characterGroup
	if json.Unmarshal([]byte(fyne.CurrentApp().Preferences().String(characterGroupsPreference)), &saved) != nil {
		return
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, group := range saved {
		group.Name = strings.TrimSpace(group.Name)
		if group.ID == "" || group.Name == "" || ids[group.ID] || names[characterKey(group.Name)] || strings.EqualFold(group.Name, ungroupedCharacters) {
			continue
		}
		group.Members = cleanGroupMembers(group.Members)
		ui.characterGroups = append(ui.characterGroups, group)
		ids[group.ID], names[characterKey(group.Name)] = true, true
	}
	ui.sortCharacterGroups()
}

func (ui *analyzerUI) sortCharacterGroups() {
	sort.Slice(ui.characterGroups, func(i, j int) bool {
		return compareTotalsText(ui.characterGroups[i].Name, ui.characterGroups[j].Name) < 0
	})
}

func (ui *analyzerUI) validateCharacterGroupName(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("enter a group name")
	}
	if strings.EqualFold(name, ungroupedCharacters) {
		return fmt.Errorf("%q is reserved for characters without a group", ungroupedCharacters)
	}
	for _, group := range ui.characterGroups {
		if group.ID != id && strings.EqualFold(group.Name, name) {
			return fmt.Errorf("a group named %q already exists", group.Name)
		}
	}
	return nil
}

func (ui *analyzerUI) saveCharacterGroup(group characterGroup) (characterGroup, error) {
	if err := ui.validateCharacterGroupName(group.ID, group.Name); err != nil {
		return group, err
	}
	if group.ID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return group, fmt.Errorf("create group: %w", err)
		}
		group.ID = hex.EncodeToString(id[:])
	}
	group.Name, group.Members = strings.TrimSpace(group.Name), cleanGroupMembers(group.Members)
	selectedID := ui.selectedCharacterGroupID()
	updated := false
	for index := range ui.characterGroups {
		if ui.characterGroups[index].ID == group.ID {
			ui.characterGroups[index] = group
			updated = true
			break
		}
	}
	if !updated {
		ui.characterGroups = append(ui.characterGroups, group)
	}
	ui.persistCharacterGroups(selectedID)
	return group, nil
}

func (ui *analyzerUI) deleteCharacterGroup(id string) {
	selectedID := ui.selectedCharacterGroupID()
	for index, group := range ui.characterGroups {
		if group.ID == id {
			ui.characterGroups = append(ui.characterGroups[:index], ui.characterGroups[index+1:]...)
			break
		}
	}
	ui.persistCharacterGroups(selectedID)
}

func (ui *analyzerUI) persistCharacterGroups(selectedID string) {
	ui.sortCharacterGroups()
	data, _ := json.Marshal(ui.characterGroups)
	fyne.CurrentApp().Preferences().SetString(characterGroupsPreference, string(data))
	ui.groupSelect.Options = ui.characterGroupingOptions()
	if selectedID != "" {
		ui.groupSelect.Selected = "Characters"
		for _, group := range ui.characterGroups {
			if group.ID == selectedID {
				ui.groupSelect.Selected = "Group: " + group.Name
			}
		}
	}
	ui.groupSelect.Refresh()
	ui.saveGraphFormatPreferences()
	ui.rebuildView()
}

func (ui *analyzerUI) characterGroupingOptions() []string {
	options := []string{"Characters", "Ship history", "Ship class history", characterGroupsMode, filterWeaponType, filterWeaponStrength, filterTarget, filterTargetType, filterSystem}
	for _, group := range ui.characterGroups {
		options = append(options, "Group: "+group.Name)
	}
	return options
}

func (ui *analyzerUI) selectedCharacterGroupID() string {
	if ui.groupSelect != nil {
		for _, group := range ui.characterGroups {
			if ui.groupSelect.Selected == "Group: "+group.Name {
				return group.ID
			}
		}
	}
	return ""
}

func (ui *analyzerUI) characterGroupGrouping() bool {
	return ui.groupSelect != nil && (ui.groupSelect.Selected == characterGroupsMode || ui.selectedCharacterGroupID() != "")
}

func (ui *analyzerUI) anyGrouping() bool {
	return ui.shipGrouping() || ui.characterGroupGrouping() || ui.damageAttributeGrouping() || ui.systemGrouping()
}

func (ui *analyzerUI) characterMatchesGroupFilter(name string) bool {
	id := ui.selectedCharacterGroupID()
	if id == "" {
		return true
	}
	for _, group := range ui.characterGroups {
		if group.ID == id {
			return group.contains(name)
		}
	}
	return false
}

// A named outgoing group limits only outgoing data, never another graph's
// selected characters or the session summary.
func (ui *analyzerUI) outgoingCharacterSelection() map[string]bool {
	id := ui.selectedCharacterGroupID()
	if id == "" {
		return ui.selected
	}
	selected := map[string]bool{}
	for _, group := range ui.characterGroups {
		if group.ID == id {
			for name, checked := range ui.selected {
				if checked && group.contains(name) {
					selected[name] = true
				}
			}
		}
	}
	return selected
}

func (ui *analyzerUI) groupCustomCharacterSeries(series []characterSeries, selected map[string]bool) []characterSeries {
	return ui.groupCharacterSeriesFor(series, selected, ui.selectedCharacterGroupID())
}

func (ui *analyzerUI) groupCharacterSeriesFor(series []characterSeries, selected map[string]bool, filterID string) []characterSeries {
	return ui.groupCharacterSeriesIn(graphOutgoing, series, selected, filterID)
}
func (ui *analyzerUI) groupCharacterSeriesIn(scope string, series []characterSeries, selected map[string]bool, filterID string) []characterSeries {
	groups := map[string]*characterSeries{}
	memberGroups := map[string][]string{}
	for _, group := range ui.characterGroups {
		if filterID != "" && filterID != group.ID {
			continue
		}
		if !ui.globalFilter(filterCharacterGroup).selected("group:"+group.ID) || !ui.scopeFilter(scope, filterCharacterGroup).selected("group:"+group.ID) {
			continue
		}
		for _, name := range group.Members {
			memberGroups[characterKey(name)] = append(memberGroups[characterKey(name)], group.Name)
		}
	}
	for _, character := range series {
		if selected != nil && !selected[character.Name] {
			continue
		}
		names := memberGroups[characterKey(character.Name)]
		if len(names) == 0 && filterID == "" && ui.globalFilter(filterCharacterGroup).selected(filterUngrouped) && ui.scopeFilter(scope, filterCharacterGroup).selected(filterUngrouped) {
			names = []string{ungroupedCharacters}
		}
		for _, name := range names {
			group := groups[name]
			if group == nil {
				group = &characterSeries{Name: name, Incoming: make([]float64, len(character.Incoming)), Outgoing: make([]float64, len(character.Outgoing)), MemberShips: map[string][]string{}}
				groups[name] = group
			}
			group.Members = append(group.Members, character.Name)
			group.MemberShips[character.Name] = []string{shipLabel(character.Name, ui.ships)}
			group.TotalIncoming += character.TotalIncoming
			group.TotalOutgoing += character.TotalOutgoing
			group.IncomingHits += character.IncomingHits
			group.OutgoingHits += character.OutgoingHits
			group.IncomingMisses = group.IncomingMisses || character.IncomingMisses
			group.OutgoingMisses = group.OutgoingMisses || character.OutgoingMisses
			for index, value := range character.Incoming {
				group.Incoming[index] += value
			}
			for index, value := range character.Outgoing {
				group.Outgoing[index] += value
			}
		}
	}
	var result []characterSeries
	for _, group := range groups {
		sort.Slice(group.Members, func(i, j int) bool { return compareTotalsText(group.Members[i], group.Members[j]) < 0 })
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool { return compareTotalsText(result[i].Name, result[j].Name) < 0 })
	return result
}

// Non-damage notifications count too. Retain unique entry seconds during parsing
// so a log covering a long interval does not imply activity throughout its gaps.
func charactersWithLogEntries(snapshot *logSnapshot, start, end time.Time) map[string]string {
	names := map[string]string{}
	if !end.After(start) {
		return names
	}
	first, last := start.Unix(), end.Unix()
	for _, log := range snapshot.Logs {
		found := false
		if len(log.EntrySeconds) > 0 {
			index := sort.Search(len(log.EntrySeconds), func(index int) bool { return log.EntrySeconds[index] >= first })
			found = index < len(log.EntrySeconds) && log.EntrySeconds[index] < last
		} else {
			for _, point := range log.Points {
				found = found || (point.Second >= first && point.Second < last)
			}
			for _, event := range log.StructureEvents {
				found = found || (event.Second >= first && event.Second < last)
			}
		}
		if found && strings.TrimSpace(log.Character) != "" {
			names[characterKey(log.Character)] = log.Character
		}
	}
	return names
}

func (ui *analyzerUI) systemGrouping() bool {
	return ui.groupSelect != nil && ui.groupSelect.Selected == filterSystem
}
