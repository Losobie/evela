package main

import (
	"log"
	"os"
	"slices"

	"fyne.io/fyne/v2"
)

func main() {
	if restoreTrayInstance() {
		return
	}
	captureRuntimeErrors()
	stopProfiling, err := startPerformanceProfiler(os.Getenv("EVELA_PROFILE_ADDR"))
	if err != nil {
		log.Printf("Performance profiler unavailable: %v", err)
	} else {
		defer stopProfiling()
	}

	application := newEvelaApp()
	applySavedAppearance(application)
	window := application.NewWindow("Evela · EVE Online log analyzer")
	ui := newAnalyzerUI(window)
	window.SetContent(ui.content())
	window.Resize(fyne.NewSize(1440, 960))
	window.SetOnClosed(ui.saveWindowGeometry)
	defer func() {
		ui.cancelForeground()
		ui.engagements.cancelBuild()
		close(ui.stop)
	}()
	windowInitialized := false
	initializeWindow := func() {
		if windowInitialized {
			return
		}
		windowInitialized = true
		ui.restoreWindowGeometry()
		ui.initGraphMouseNavigation()
	}
	minimizeToTray, stopTrayRestore := ui.setupSystemTray(application, initializeWindow)
	if stopTrayRestore != nil {
		defer stopTrayRestore()
	}
	startInTray := minimizeToTray != nil && slices.Contains(os.Args[1:], startInTrayArg)
	if startInTray {
		minimizeToTray()
	} else {
		window.Show()
	}
	go ui.watch()
	go ui.watchESI()
	ui.requestLoad()
	application.Lifecycle().SetOnStarted(func() {
		if !startInTray {
			initializeWindow()
		}
		go ui.watchReleases()
	})
	application.Run()
}
