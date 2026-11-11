//go:build ci

package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// Run in a fresh process with:
// go test -tags ci -run '^TestFyneDoStartup$' .
// This exercises production app creation and app.Run (including the migration
// warning), using Fyne's software driver instead of opening a native window.
func TestFyneDoStartup(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	previous := log.Writer()
	defer log.SetOutput(previous)
	var output bytes.Buffer
	log.SetOutput(&output)
	application := newEvelaApp()
	if !application.Metadata().Migrations["fyneDo"] || application.UniqueID() != applicationID || application.Metadata().Icon == nil {
		t.Fatal("production startup lost migration, identity or icon metadata")
	}
	application.Run()
	application.Quit()
	if strings.Contains(output.String(), "has not been migrated") || strings.Contains(output.String(), "Error in Fyne call thread") {
		t.Fatalf("startup triggered Fyne's threading compatibility warning: %s", output.String())
	}
}
