package main

import (
	"encoding/json"
	"slices"

	"fyne.io/fyne/v2"
)

const graphRangePreference = "graph.range.v1"
const defaultGraphRangePreset = "Last 30 minutes"

type graphRangePreferenceState struct {
	Preset string `json:"preset"`
	Start  string `json:"start,omitempty"`
	End    string `json:"end,omitempty"`
}

func (ui *analyzerUI) loadGraphRangePreference() {
	var saved graphRangePreferenceState
	data := fyne.CurrentApp().Preferences().String(graphRangePreference)
	if json.Unmarshal([]byte(data), &saved) != nil {
		saved = graphRangePreferenceState{}
	}
	// Preserve exact bounds saved before the custom option was renamed.
	if saved.Preset == "Custom UTC range" {
		saved.Preset = "Custom"
	}
	if !slices.Contains(ui.rangeSelect.Options, saved.Preset) {
		saved = graphRangePreferenceState{Preset: defaultGraphRangePreset}
	}
	if saved.Preset == "Custom" {
		if _, _, err := parseRange(saved.Start, saved.End); err != nil {
			saved = graphRangePreferenceState{Preset: defaultGraphRangePreset}
		} else {
			ui.settingRange = true
			ui.start.SetText(saved.Start)
			ui.end.SetText(saved.End)
			ui.settingRange = false
		}
	}
	ui.rangeSelect.SetSelected(saved.Preset)
}

func (ui *analyzerUI) saveGraphRangePreference() {
	saved := graphRangePreferenceState{Preset: ui.rangeSelect.Selected}
	if saved.Preset == "Custom" {
		saved.Start = ui.view.Start.UTC().Format("2006-01-02 15:04:05")
		saved.End = ui.view.End.UTC().Format("2006-01-02 15:04:05")
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return
	}
	preferences := fyne.CurrentApp().Preferences()
	// Refreshes rebuild the view without changing the
	// selected range. Avoid rewriting the preference for those updates.
	if preferences.String(graphRangePreference) != string(data) {
		preferences.SetString(graphRangePreference, string(data))
	}
}
