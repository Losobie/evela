package main

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsStartupCommand(t *testing.T) {
	path := `C:\Program Files\Evela\evela.exe`
	for _, minimized := range []bool{false, true} {
		command, err := windowsStartupCommand(path, minimized)
		if err != nil {
			t.Fatal(err)
		}
		args, err := windows.DecomposeCommandLine(command)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{path}
		if minimized {
			want = append(want, startInTrayArg)
		}
		if !slices.Equal(args, want) {
			t.Fatalf("startup command parsed as %q, want %q", args, want)
		}
	}
	if _, err := windowsStartupCommand(path+strings.Repeat("x", 260), false); err == nil {
		t.Fatal("expected an error for an oversized Windows startup command")
	}
}
