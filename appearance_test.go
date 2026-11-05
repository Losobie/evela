package main

import (
	"image/color"
	"os"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestAppearanceDefaultAndSystemVariants(t *testing.T) {
	application := test.NewTempApp(t)
	if savedAppearance(application) != appearanceSystem {
		t.Fatal("new installations must follow the system")
	}
	for _, choice := range []string{appearanceSystem, appearanceLight, appearanceDark, "invalid"} {
		application.Preferences().SetString(appearancePreference, choice)
		applySavedAppearance(application)
		for _, variant := range []fyne.ThemeVariant{theme.VariantLight, theme.VariantDark} {
			wantVariant := variant
			switch choice {
			case appearanceLight:
				wantVariant = theme.VariantLight
			case appearanceDark:
				wantVariant = theme.VariantDark
			}
			for _, name := range []fyne.ThemeColorName{theme.ColorNameBackground, theme.ColorNameForeground, theme.ColorNamePlaceHolder, theme.ColorNameDisabled, theme.ColorNameInputBackground} {
				got := application.Settings().Theme().Color(name, variant)
				want := theme.DefaultTheme().Color(name, wantVariant)
				if wantVariant == theme.VariantLight && (name == theme.ColorNameForeground || name == theme.ColorNamePlaceHolder) {
					r, g, b, a := got.RGBA()
					wr, wg, wb, wa := want.RGBA()
					if r > wr || g > wg || b > wb || r+g+b >= wr+wg+wb || a != wa {
						t.Fatalf("light-mode text was not darkened: %v versus %v", got, want)
					}
					continue
				}
				if got != want {
					t.Fatalf("choice %q, color %s, variant %d: got %v, want %v", choice, name, variant, got, want)
				}
			}
		}
	}
	if savedAppearance(application) != appearanceSystem {
		t.Fatal("invalid saved appearance must fall back to System")
	}
}

func TestAppearanceMenuUpdatesChartsAndPersists(t *testing.T) {
	ui, _ := newDamageFilterTestUI(t)
	application := fyne.CurrentApp()
	chart := ui.outgoing
	renderer := test.WidgetRenderer(chart).(*chartRenderer)
	before := chart.data
	swatch := newSeriesSwatch(before.Series[0].Name)
	swatchRenderer := test.WidgetRenderer(swatch).(*seriesSwatchRenderer)
	groupRow := newCharacterGroupRow()
	groupRow.setSelected(true)
	groupRenderer := test.WidgetRenderer(groupRow)
	baseColor := before.Series[0].Color
	for _, choice := range []string{appearanceLight, appearanceDark, appearanceSystem} {
		menu := ui.window.MainMenu().Items[0].Items[0].ChildMenu
		if menu.Label != "Theme" || len(menu.Items) != 3 {
			t.Fatal("Settings must offer System, Light and Dark themes")
		}
		for _, item := range menu.Items {
			if item.Label == choice {
				item.Action()
				break
			}
		}
		if savedAppearance(application) != choice {
			t.Fatalf("theme choice %s was not saved", choice)
		}
		if menus := ui.window.MainMenu().Items; len(menus) != 2 || menus[0].Label != "Settings" || menus[1].Label != "About" {
			t.Fatal("switching themes must preserve Settings and About menus")
		}
		for _, item := range ui.window.MainMenu().Items[0].Items[0].ChildMenu.Items {
			if item.Checked != (item.Label == choice) {
				t.Fatal("theme menu checkmarks do not reflect the selection")
			}
		}
		// A native settings change refreshes every window's widgets. Exercise the
		// same renderers explicitly with the test driver, which has no OS watcher.
		ui.window.Content().Refresh()
		renderer.Refresh()
		swatchRenderer.Refresh()
		groupRenderer.Refresh()
		palette := currentChartPalette()
		if choice != appearanceSystem && palette.light != (choice == appearanceLight) {
			t.Fatalf("chart did not adopt the %s theme", choice)
		}
		if got := color.NRGBAModel.Convert(chart.renderPlot(100, 100).At(2, 2)); got != palette.background {
			t.Fatalf("raster background did not follow the theme: %v", got)
		}
		chart.MouseMoved(testMouseEvent(chartPlotLeft+20, 60, 0))
		if chart.hoverReadings[0].Color != palette.seriesColor(baseColor) || renderer.yLabels[0].Color != palette.text || swatchRenderer.rectangle.FillColor != palette.seriesColor(baseColor) {
			t.Fatal("hover, axis labels or character colors did not follow the theme")
		}
		if groupRow.background.FillColor != theme.Color(theme.ColorNameSelection) || !groupRow.background.Visible() {
			t.Fatal("character group selection did not follow the theme")
		}
		if !reflect.DeepEqual(chart.data, before) {
			t.Fatal("switching themes changed the current graph data")
		}
		// Simulate startup after replacing the active theme, keeping preferences.
		application.Settings().SetTheme(theme.DefaultTheme())
		applySavedAppearance(application)
		if got := application.Settings().Theme().(appearanceTheme).choice; got != choice {
			t.Fatalf("startup restored %s instead of %s", got, choice)
		}
		if prefix := os.Getenv("EVELA_APPEARANCE_CAPTURE"); prefix != "" {
			captureWindowPreview(t, ui.window, prefix+"."+choice+".png")
		}
	}
}
