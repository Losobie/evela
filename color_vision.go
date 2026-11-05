package main

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
)

const colorVisionPreference = "ui.graph-color-vision.v1"
const visionDefault = "Default"
const visionProtanopia = "Protanopia (red-blind)"
const visionDeuteranopia = "Deuteranopia (green-blind)"
const visionTritanopia = "Tritanopia (blue-blind)"
const visionAchromatopsia = "Achromatopsia (monochrome)"
const graphColorContrast = 4.6 // Leave a margin for rounding channels to bytes.

var colorVisionChoices = []string{visionDefault, visionProtanopia, visionDeuteranopia, visionTritanopia, visionAchromatopsia}

func savedColorVision(application fyne.App) string {
	choice := application.Preferences().StringWithFallback(colorVisionPreference, visionDefault)
	for _, known := range colorVisionChoices {
		if choice == known {
			return choice
		}
	}
	return visionDefault
}

func (ui *analyzerUI) colorVisionMenu() *fyne.Menu {
	menu := fyne.NewMenu("Graph colors")
	for _, choice := range colorVisionChoices {
		choice := choice
		item := fyne.NewMenuItem(choice, func() {
			application := fyne.CurrentApp()
			application.Preferences().SetString(colorVisionPreference, choice)
			// Reapply the theme to refresh graphs, hover indicators and swatches
			// through their renderers, without rebuilding queries or parsed data.
			applySavedAppearance(application)
			ui.window.SetMainMenu(ui.mainMenu())
		})
		item.Checked = choice == savedColorVision(fyne.CurrentApp())
		menu.Items = append(menu.Items, item)
	}
	return menu
}

// Full-severity linear-sRGB simulation matrices from Machado, Oliveira and
// Fernandes (2009), A Physiologically-based Model for Simulation of Color
// Vision Deficiency. Used to identify lost information, not to simulate the
// deficiency on screen: that error is redistributed into other channels.
// https://www.inf.ufrgs.br/~oliveira/pubs_files/CVD_Simulation/CVD_Simulation.html
func colorVisionMatrix(choice string) ([3][3]float64, int) {
	switch choice {
	case visionProtanopia:
		return [3][3]float64{{.152286, 1.052583, -.204868}, {.114503, .786281, .099216}, {-.003882, -.048116, 1.051998}}, 0
	case visionDeuteranopia:
		return [3][3]float64{{.367322, .860646, -.227968}, {.280085, .672501, .047413}, {-.011820, .042940, .968881}}, 1
	case visionTritanopia:
		return [3][3]float64{{1.255528, -.076749, -.178779}, {-.078411, .930809, .147602}, {.004733, .691367, .303900}}, 2
	}
	return [3][3]float64{}, -1
}

func linearColor(channel uint8) float64 {
	value := float64(channel) / 255
	if value <= .04045 {
		return value / 12.92
	}
	return math.Pow((value+.055)/1.055, 2.4)
}

func encodedColor(value float64) uint8 {
	value = math.Max(0, math.Min(1, value))
	if value <= .0031308 {
		return uint8(math.Round(255 * 12.92 * value))
	}
	return uint8(math.Round(255 * (1.055*math.Pow(value, 1/2.4) - .055)))
}

func relativeLuminance(rgb [3]float64) float64 {
	return .2126*rgb[0] + .7152*rgb[1] + .0722*rgb[2]
}

func (palette chartPalette) accessibleColor(ink color.NRGBA) color.NRGBA {
	rgb := [3]float64{linearColor(ink.R), linearColor(ink.G), linearColor(ink.B)}
	if palette.vision == visionAchromatopsia {
		luminance := relativeLuminance(rgb)
		// Encode hue as brightness, rather than simply desaturating: two
		// equally bright colors should not collapse to the same gray.
		r, g, b := float64(ink.R), float64(ink.G), float64(ink.B)
		high, low := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
		if delta := high - low; delta > 0 {
			var hue float64
			switch high {
			case r:
				hue = (g - b) / delta
			case g:
				hue = (b-r)/delta + 2
			default:
				hue = (r-g)/delta + 4
			}
			if hue < 0 {
				hue += 6
			}
			luminance = .20 + .65*hue/6
		}
		rgb = [3]float64{luminance, luminance, luminance}
	} else {
		matrix, missing := colorVisionMatrix(palette.vision)
		var difference [3]float64
		for row := range matrix {
			difference[row] = rgb[row]
			for column := range rgb {
				difference[row] -= matrix[row][column] * rgb[column]
			}
		}
		if missing >= 0 {
			for channel := range rgb {
				if channel != missing {
					rgb[channel] += difference[channel] + .7*difference[missing]
				}
				rgb[channel] = math.Max(0, math.Min(1, rgb[channel]))
			}
		}
	}
	// Fit the corrected color to the plot background. Linear-light blending
	// preserves hue differences while giving opaque colors >= 4.5:1 contrast.
	background := relativeLuminance([3]float64{linearColor(palette.background.R), linearColor(palette.background.G), linearColor(palette.background.B)})
	if palette.light {
		for channel := range rgb {
			rgb[channel] = linearColor(uint8(float64(encodedColor(rgb[channel])) * .45))
		}
		maximum := (background+.05)/graphColorContrast - .05
		if luminance := relativeLuminance(rgb); luminance > maximum {
			for channel := range rgb {
				rgb[channel] *= maximum / luminance
			}
		}
	} else {
		minimum := (background+.05)*graphColorContrast - .05
		if luminance := relativeLuminance(rgb); luminance < minimum {
			blend := (minimum - luminance) / (1 - luminance)
			for channel := range rgb {
				rgb[channel] += (1 - rgb[channel]) * blend
			}
		}
	}
	return color.NRGBA{encodedColor(rgb[0]), encodedColor(rgb[1]), encodedColor(rgb[2]), ink.A}
}
