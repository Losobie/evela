package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEmbeddedLicensesCoverPinnedDependenciesAndAssets(t *testing.T) {
	if !strings.Contains(projectLicense, "MIT License") || !strings.Contains(projectLicense, "2026 Losobie") {
		t.Fatal("Evela's MIT license is missing from the executable resources")
	}
	data, err := exec.Command("go", "mod", "edit", "-json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var module struct {
		Require []struct{ Path, Version string }
	}
	if err := json.Unmarshal(data, &module); err != nil {
		t.Fatal(err)
	}
	documents := map[string]string{}
	for _, document := range licenseDocuments() {
		if document.name == "" || document.text == "" || documents[document.name] != "" {
			t.Fatalf("empty or duplicate license section: %q", document.name)
		}
		documents[document.name] = document.text
	}
	for _, dependency := range module.Require {
		if documents[dependency.Path+" "+dependency.Version] == "" {
			t.Errorf("no embedded notice for %s %s", dependency.Path, dependency.Version)
		}
	}
	for _, required := range []string{"SIL OPEN FONT LICENSE", "Bitstream Vera", "HarfBuzz", "UNICODE LICENSE V3", "GCC RUNTIME LIBRARY EXCEPTION", "GNU LESSER GENERAL PUBLIC LICENSE", "CCP", "https://developers.eveonline.com/license-agreement"} {
		if !strings.Contains(strings.ToLower(thirdPartyNotices), strings.ToLower(required)) {
			t.Errorf("missing third-party attribution / terms: %s", required)
		}
	}
}

func TestLicensesAreAccessibleFromAbout(t *testing.T) {
	ui, _ := newEventsTestUI(t)
	found := false
	for _, item := range ui.window.MainMenu().Items[1].Items {
		if item.Label == "Licenses" {
			item.Action()
			found = true
			break
		}
	}
	if !found || ui.window.Canvas().Overlays().Top() == nil {
		t.Fatal("standalone executable has no accessible license viewer")
	}
	captureWindowPreview(t, ui.window, os.Getenv("EVELA_LICENSES_CAPTURE"))
}
