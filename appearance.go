package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const appearancePreference = "ui.appearance.v1"
const appearanceSystem = "System"
const appearanceLight = "Light"
const appearanceDark = "Dark"

// System leaves the framework's native theme variant intact, including live
// system changes. An explicit preference overrides colors without changing data.
type appearanceTheme struct {
	fyne.Theme
	choice string
}

func (appearance appearanceTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch appearance.choice {
	case appearanceLight:
		variant = theme.VariantLight
	case appearanceDark:
		variant = theme.VariantDark
	}
	base := appearance.Theme.Color(name, variant)
	if variant == theme.VariantLight && (name == theme.ColorNameForeground || name == theme.ColorNamePlaceHolder) {
		ink := color.NRGBAModel.Convert(base).(color.NRGBA)
		factor := 0.75
		if name == theme.ColorNamePlaceHolder {
			factor = 0.85
		}
		return color.NRGBA{uint8(float64(ink.R) * factor), uint8(float64(ink.G) * factor), uint8(float64(ink.B) * factor), ink.A}
	}
	return base
}
func savedAppearance(application fyne.App) string {
	choice := application.Preferences().StringWithFallback(appearancePreference, appearanceSystem)
	switch choice {
	case appearanceLight, appearanceDark:
		return choice
	default:
		return appearanceSystem
	}
}
func applySavedAppearance(application fyne.App) {
	application.Settings().SetTheme(appearanceTheme{Theme: theme.DefaultTheme(), choice: savedAppearance(application)})
}
func (ui *analyzerUI) appearanceMenu() *fyne.Menu {
	menu := fyne.NewMenu("Theme")
	for _, choice := range []string{appearanceSystem, appearanceLight, appearanceDark} {
		choice := choice
		item := fyne.NewMenuItem(choice, func() {
			application := fyne.CurrentApp()
			application.Preferences().SetString(appearancePreference, choice)
			applySavedAppearance(application)
			ui.window.SetMainMenu(fyne.NewMainMenu(ui.settingsMenu()))
		})
		item.Checked = choice == savedAppearance(fyne.CurrentApp())
		menu.Items = append(menu.Items, item)
	}
	return menu
}
func (ui *analyzerUI) settingsMenu() *fyne.Menu {
	appearance := fyne.NewMenuItem("Theme", nil)
	appearance.ChildMenu = ui.appearanceMenu()
	return fyne.NewMenu("Settings", appearance, fyne.NewMenuItem("Log loading…", ui.openLogLoadingSettings), fyne.NewMenuItem("Character groups…", ui.openCharacterGroupSettings), fyne.NewMenuItem("ESI configuration…", ui.openESISettings))
}

type chartPalette struct {
	light                              bool
	background, grid, text, cursor     color.NRGBA
	selection, selectionStroke, marker color.NRGBA
}

func currentChartPalette() chartPalette {
	palette := chartPalette{
		background: color.NRGBA{17, 23, 33, 255}, grid: color.NRGBA{42, 53, 69, 255},
		text: color.NRGBA{150, 166, 187, 255}, cursor: color.NRGBA{170, 185, 200, 180},
		selection: color.NRGBA{68, 209, 223, 55}, selectionStroke: color.NRGBA{68, 209, 223, 220},
		marker: color.NRGBA{255, 70, 70, 255},
	}
	application := fyne.CurrentApp()
	if application == nil {
		return palette
	}
	background := application.Settings().Theme().Color(theme.ColorNameBackground, application.Settings().ThemeVariant())
	r, g, b, _ := background.RGBA()
	if (uint64(r)*299+uint64(g)*587+uint64(b)*114)/1000 < 32768 {
		return palette
	}
	return chartPalette{
		light:      true,
		background: color.NRGBA{255, 255, 255, 255}, grid: color.NRGBA{222, 229, 238, 255},
		text: color.NRGBA{49, 61, 77, 255}, cursor: color.NRGBA{83, 100, 120, 200},
		selection: color.NRGBA{0, 113, 159, 40}, selectionStroke: color.NRGBA{0, 113, 159, 220},
		marker: color.NRGBA{198, 32, 38, 255},
	}
}
func (palette chartPalette) seriesColor(ink color.NRGBA) color.NRGBA {
	if !palette.light {
		return ink
	}
	// Keep the stable series hue, with enough contrast for labels on light cards.
	return color.NRGBA{uint8(float64(ink.R) * 0.45), uint8(float64(ink.G) * 0.45), uint8(float64(ink.B) * 0.45), ink.A}
}

type seriesSwatch struct {
	widget.BaseWidget
	name string
}

func newSeriesSwatch(name string) *seriesSwatch {
	swatch := &seriesSwatch{name: name}
	swatch.ExtendBaseWidget(swatch)
	return swatch
}
func (swatch *seriesSwatch) CreateRenderer() fyne.WidgetRenderer {
	rectangle := canvas.NewRectangle(currentChartPalette().seriesColor(characterColor(swatch.name)))
	rectangle.SetMinSize(fyne.NewSize(6, 18))
	return &seriesSwatchRenderer{WidgetRenderer: widget.NewSimpleRenderer(rectangle), swatch: swatch, rectangle: rectangle}
}

type seriesSwatchRenderer struct {
	fyne.WidgetRenderer
	swatch    *seriesSwatch
	rectangle *canvas.Rectangle
}

func (renderer *seriesSwatchRenderer) Refresh() {
	renderer.rectangle.FillColor = currentChartPalette().seriesColor(characterColor(renderer.swatch.name))
	renderer.WidgetRenderer.Refresh()
}
