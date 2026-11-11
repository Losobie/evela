package main

import (
	"log"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	captureRuntimeErrors()
	stopProfiling, err := startPerformanceProfiler(os.Getenv("EVELA_PROFILE_ADDR"))
	if err != nil {
		log.Printf("Performance profiler unavailable: %v", err)
	} else {
		defer stopProfiling()
	}

	application := app.NewWithID("io.evela.damage-analyzer")
	application.SetIcon(fyne.NewStaticResource("icon.svg", applicationIcon))
	applySavedAppearance(application)
	window := application.NewWindow("Evela · EVE Online log analyzer")
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.SetOnClosed(func() {
		ui.saveWindowGeometry()
		ui.engagements.cancelBuild()
		close(ui.stop)
	})
	go ui.watch()
	go ui.watchESI()
	ui.requestLoad()
	application.Lifecycle().SetOnStarted(func() {
		ui.restoreWindowGeometry()
		ui.initGraphMouseNavigation()
		go ui.watchReleases()
	})
	window.ShowAndRun()
}
