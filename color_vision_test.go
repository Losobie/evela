package main

import (
	"image/color"
	"math"
	"os"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func colorLuminance(ink color.NRGBA) float64 {
	return relativeLuminance([3]float64{linearColor(ink.R), linearColor(ink.G), linearColor(ink.B)})
}

func TestColorVisionContrastAndConfusableColors(t *testing.T) {
	application := test.NewTempApp(t)
	for _, appearance := range []string{appearanceLight, appearanceDark} {
		application.Preferences().SetString(appearancePreference, appearance)
		applySavedAppearance(application)
		for _, choice := range colorVisionChoices[1:] {
			application.Preferences().SetString(colorVisionPreference, choice)
			palette := currentChartPalette()
			for i := 0; i < 1000; i++ {
				ink := palette.seriesColor(characterColor(string(rune(i + 32))))
				foreground, background := colorLuminance(ink), colorLuminance(palette.background)
				contrast := (math.Max(foreground, background) + .05) / (math.Min(foreground, background) + .05)
				if contrast < 4.5 {
					t.Fatalf("%s/%s: color %v has only %.3f:1 contrast", appearance, choice, ink, contrast)
				}
				if choice == visionAchromatopsia && (ink.R != ink.G || ink.G != ink.B) {
					t.Fatal("monochrome mode produced a colored series")
				}
			}
			if alpha := palette.seriesColor(color.NRGBA{240, 110, 90, 130}).A; alpha != 130 {
				t.Fatal("color correction changed transparency")
			}
		}
	}

	// These well-separated original hues nearly coincide after full-severity
	// simulation. Correction must increase their simulated separation, rather
	// than merely show the user a color-blindness simulation of the originals.
	for _, pair := range []struct {
		mode string
		a, b color.NRGBA
	}{
		{visionProtanopia, color.NRGBA{245, 230, 97, 255}, color.NRGBA{97, 245, 97, 255}},
		{visionDeuteranopia, color.NRGBA{97, 171, 245, 255}, color.NRGBA{230, 97, 245, 255}},
		{visionTritanopia, color.NRGBA{245, 141, 97, 255}, color.NRGBA{245, 97, 201, 255}},
	} {
		matrix, _ := colorVisionMatrix(pair.mode)
		distance := func(a, b color.NRGBA) float64 {
			difference := [3]float64{linearColor(a.R) - linearColor(b.R), linearColor(a.G) - linearColor(b.G), linearColor(a.B) - linearColor(b.B)}
			var square float64
			for _, row := range matrix {
				value := row[0]*difference[0] + row[1]*difference[1] + row[2]*difference[2]
				square += value * value
			}
			return math.Sqrt(square)
		}
		for _, appearance := range []string{appearanceDark, appearanceLight} {
			application.Preferences().SetString(appearancePreference, appearance)
			application.Preferences().SetString(colorVisionPreference, visionDefault)
			applySavedAppearance(application)
			original := currentChartPalette()
			application.Preferences().SetString(colorVisionPreference, pair.mode)
			corrected := currentChartPalette()
			before := distance(original.seriesColor(pair.a), original.seriesColor(pair.b))
			after := distance(corrected.seriesColor(pair.a), corrected.seriesColor(pair.b))
			if after <= before*1.5 {
				t.Fatalf("%s/%s: confusable colors were not separated: %.4f -> %.4f", appearance, pair.mode, before, after)
			}
		}
	}
}

func TestColorVisionMenuRefreshesGraphAndPersists(t *testing.T) {
	chart, window, data := newLineHoverTestChart(t)
	application := fyne.CurrentApp()
	ui := &analyzerUI{window: window}
	window.SetMainMenu(ui.mainMenu())
	swatch := newSeriesSwatch(data.Series[4].Name)
	swatchRenderer := test.WidgetRenderer(swatch).(*seriesSwatchRenderer)
	before := chart.data
	for _, appearance := range []string{appearanceDark, appearanceLight} {
		application.Preferences().SetString(appearancePreference, appearance)
		applySavedAppearance(application)
		for index, choice := range colorVisionChoices {
			menu := window.MainMenu().Items[0].Items[0].ChildMenu.Items[4].ChildMenu
			menu.Items[index].Action()
			chart.Refresh()
			swatchRenderer.Refresh()
			if savedColorVision(application) != choice || savedAppearance(application) != appearance {
				t.Fatal("graph-color selection was not saved independently of light/dark appearance")
			}
			for _, item := range ui.colorVisionMenu().Items {
				if item.Checked != (item.Label == choice) {
					t.Fatal("graph color menu checkmarks do not reflect the selection")
				}
			}
			position := hoverLinePosition(chart, .4, data.Series[4], 20)
			chart.MouseMoved(testMouseEvent(position.X, position.Y, 0))
			ink := currentChartPalette().seriesColor(data.Series[4].Color)
			if chart.hoverDot.FillColor != ink || chart.hoverReadings[0].Color != ink || swatchRenderer.rectangle.FillColor != ink {
				t.Fatal("hover dot, reading and swatch no longer match their line")
			}
			if !reflect.DeepEqual(chart.data, before) {
				t.Fatal("changing graph colors modified graph data or time range")
			}
			if prefix := os.Getenv("EVELA_COLOR_VISION_CAPTURE"); prefix != "" {
				captureWindowPreview(t, window, prefix+"."+appearance+"."+string(rune('0'+index))+".png")
			}
		}
	}
	application.Preferences().SetString(colorVisionPreference, "invalid")
	if savedColorVision(application) != visionDefault {
		t.Fatal("unknown saved modes must fall back to the original palette")
	}
}
