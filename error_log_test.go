package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

type unavailableLogOutput struct{}

func (unavailableLogOutput) Write([]byte) (int, error) { return 0, errors.New("no console") }

func TestRuntimeErrorCaptureWithoutConsole(t *testing.T) {
	previous := log.Writer()
	defer log.SetOutput(previous)
	log.SetOutput(unavailableLogOutput{})
	captureRuntimeErrors()
	reportRuntimeError("Error log capture test", errors.New("source file unavailable"))
	fyne.LogError("Fyne capture test", errors.New("window error"))
	text := runtimeErrors.Text()
	if !strings.Contains(text, "Error log capture test: source file unavailable") || !strings.Contains(text, "Fyne capture test") || !strings.Contains(text, "window error") {
		t.Fatalf("GUI session lost errors without console output: %s", text)
	}
}

func TestRuntimeErrorLogConcurrentWrites(t *testing.T) {
	var store runtimeErrorLog
	logger := log.New(&store, "", 0)
	var workers sync.WaitGroup
	for i := 0; i < 30; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			logger.Printf("failure %d", i)
			_ = store.Text()
		}(i)
	}
	workers.Wait()
	text := store.Text()
	for i := 0; i < 30; i++ {
		if strings.Count(text, fmt.Sprintf("failure %d\n", i)) != 1 {
			t.Fatalf("lost or duplicated concurrent error %d: %s", i, text)
		}
	}
}

type errorLogTestWindow struct {
	fyne.Window
	clipboard fyne.Clipboard
}

func (window errorLogTestWindow) Clipboard() fyne.Clipboard { return window.clipboard }

func errorLogDialogControls(window fyne.Window) (*widget.Label, *container.Scroll, map[string]*widget.Button) {
	var text *widget.Label
	var scroll *container.Scroll
	buttons := map[string]*widget.Button{}
	var visit func(fyne.CanvasObject)
	visit = func(object fyne.CanvasObject) {
		switch object := object.(type) {
		case *fyne.Container:
			for _, child := range object.Objects {
				visit(child)
			}
		case *container.Scroll:
			if _, ok := object.Content.(*widget.Label); ok {
				scroll = object
			}
			visit(object.Content)
		case *widget.Label:
			if object.Selectable {
				text = object
			}
		case *widget.Button:
			buttons[object.Text] = object
		default:
			if object, ok := object.(fyne.Widget); ok {
				for _, child := range test.WidgetRenderer(object).Objects() {
					visit(child)
				}
			}
		}
	}
	visit(window.Canvas().Overlays().Top())
	return text, scroll, buttons
}

func TestErrorLogDialogSelectionRefreshAndCopy(t *testing.T) {
	application := test.NewTempApp(t)
	window := errorLogTestWindow{application.NewWindow("Error log test"), test.NewClipboard()}
	defer window.Close()
	window.SetContent(widget.NewLabel("Evela"))
	window.Resize(fyne.NewSize(1100, 800))
	window.Show()
	var store runtimeErrorLog
	viewer := newErrorLogDialog(window, &store, nil)
	viewer.Show()
	defer viewer.Hide()
	text, scroll, buttons := errorLogDialogControls(window)
	if text == nil || scroll == nil || text.Text != "No errors recorded during this session." {
		t.Fatal("error log must provide selectable, scrollable text and an empty state")
	}
	message := strings.Repeat("Log loading: source file unavailable\n", 80)
	_, _ = store.Write([]byte(message))
	test.Tap(buttons["Refresh"])
	window.Canvas().Capture()
	if text.Text != message {
		t.Fatal("refresh did not display new diagnostics")
	}
	test.Tap(buttons["Copy all"])
	if window.Clipboard().Content() != message {
		t.Fatal("copy all lost diagnostic text")
	}
	if text.MinSize().Height <= scroll.Size().Height {
		t.Fatal("long diagnostic text must extend beyond the scroll viewport")
	}
}

func TestSettingsErrorLogMenu(t *testing.T) {
	application := test.NewTempApp(t)
	window := errorLogTestWindow{application.NewWindow("Settings"), test.NewClipboard()}
	defer window.Close()
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Show()
	for _, item := range ui.settingsMenu().Items {
		if item.Label != "Error log" {
			continue
		}
		item.Action()
		first := ui.errorLogDialog
		if first == nil {
			t.Fatal("menu did not open error log")
		}
		item.Action()
		if ui.errorLogDialog != first {
			t.Fatal("menu opened duplicate dialogs")
		}
		text, _, buttons := errorLogDialogControls(window)
		if text == nil || strings.Contains(text.Text, "Graph pipeline") || buttons["Copy diagnostic report"] == nil {
			t.Fatal("Error log must keep diagnostics separate from session errors")
		}
		test.Tap(buttons["Copy all"])
		if window.Clipboard().Content() != text.Text {
			t.Fatal("Copy all omitted the errors")
		}
		test.Tap(buttons["Copy diagnostic report"])
		if !strings.Contains(window.Clipboard().Content(), "Graph pipeline") {
			t.Fatal("diagnostic report button did not copy the report")
		}
		first.Hide()
		if ui.errorLogDialog != nil {
			t.Fatal("closed dialog was retained")
		}
		return
	}
	t.Fatal("Settings is missing Error log")
}
