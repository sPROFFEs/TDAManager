package launcher

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ─── Palette definitions ──────────────────────────────────────────────────────
// Two palettes: light (GitHub Primer) and dark (GitHub Dark).
// Colours are applied at startup and whenever the user toggles the mode.

type palette struct {
	toolbar, sidebar, surface, border, button color.NRGBA
	text, accent, ok, warn, err, muted        color.NRGBA
}

var lightPalette = palette{
	toolbar: color.NRGBA{R: 246, G: 248, B: 250, A: 255}, // #F6F8FA – light toolbar so mode switch is visible
	sidebar: color.NRGBA{R: 249, G: 249, B: 249, A: 255}, // #F9F9F9
	surface: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // #FFFFFF
	border:  color.NRGBA{R: 222, G: 222, B: 222, A: 255}, // #DEDEDE
	button:  color.NRGBA{R: 246, G: 248, B: 250, A: 255}, // #F6F8FA
	text:    color.NRGBA{R: 27, G: 31, B: 35, A: 255},    // #1B1F23
	accent:  color.NRGBA{R: 9, G: 105, B: 218, A: 255},   // #0969DA
	ok:      color.NRGBA{R: 31, G: 136, B: 61, A: 255},   // #1F883D
	warn:    color.NRGBA{R: 191, G: 135, B: 0, A: 255},   // #BF8700
	err:     color.NRGBA{R: 207, G: 34, B: 46, A: 255},   // #CF222E
	muted:   color.NRGBA{R: 110, G: 119, B: 129, A: 255}, // #6E7781
}

var darkPalette = palette{
	toolbar: color.NRGBA{R: 22, G: 27, B: 34, A: 255},    // #161B22
	sidebar: color.NRGBA{R: 13, G: 17, B: 23, A: 255},    // #0D1117
	surface: color.NRGBA{R: 22, G: 27, B: 34, A: 255},    // #161B22
	border:  color.NRGBA{R: 48, G: 54, B: 61, A: 255},    // #30363D
	button:  color.NRGBA{R: 33, G: 38, B: 45, A: 255},    // #21262D
	text:    color.NRGBA{R: 230, G: 237, B: 243, A: 255}, // #E6EDF3
	accent:  color.NRGBA{R: 47, G: 129, B: 247, A: 255},  // #2F81F7
	ok:      color.NRGBA{R: 63, G: 185, B: 80, A: 255},   // #3FB950
	warn:    color.NRGBA{R: 210, G: 153, B: 34, A: 255},  // #D29922
	err:     color.NRGBA{R: 248, G: 81, B: 73, A: 255},   // #F85149
	muted:   color.NRGBA{R: 139, G: 148, B: 158, A: 255}, // #8B949E
}

// ─── SVG icon resources ───────────────────────────────────────────────────────
// White-filled SVGs used exclusively in HighImportance toolbar buttons
// (accent-blue background), so hardcoded white is always correct.

var moonIcon = fyne.NewStaticResource("moon.svg", []byte(
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">`+
		`<path fill="#ffffff" d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>`+
		`</svg>`))

var sunIcon = fyne.NewStaticResource("sun.svg", []byte(
	`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">`+
		`<circle fill="#ffffff" cx="12" cy="12" r="4.5"/>`+
		`<circle fill="#ffffff" cx="12" cy="3"  r="1.2"/>`+
		`<circle fill="#ffffff" cx="12" cy="21" r="1.2"/>`+
		`<circle fill="#ffffff" cx="3"  cy="12" r="1.2"/>`+
		`<circle fill="#ffffff" cx="21" cy="12" r="1.2"/>`+
		`<circle fill="#ffffff" cx="5.64"  cy="5.64"  r="1.2"/>`+
		`<circle fill="#ffffff" cx="18.36" cy="18.36" r="1.2"/>`+
		`<circle fill="#ffffff" cx="5.64"  cy="18.36" r="1.2"/>`+
		`<circle fill="#ffffff" cx="18.36" cy="5.64"  r="1.2"/>`+
		`</svg>`))

// ─── Active palette (package-level vars, reassigned on toggle) ────────────────

var (
	colorToolbar color.NRGBA
	colorSidebar color.NRGBA
	colorSurface color.NRGBA
	colorBorder  color.NRGBA
	colorButton  color.NRGBA

	colorText   color.NRGBA
	colorAccent color.NRGBA
	colorOK     color.NRGBA
	colorWarn   color.NRGBA
	colorErr    color.NRGBA
	colorMuted  color.NRGBA

	// Provider colours never change with theme.
	colorGitHub = color.NRGBA{R: 36, G: 41, B: 47, A: 255}  // #242B2F
	colorGitea  = color.NRGBA{R: 96, G: 158, B: 70, A: 255} // #609E46
)

// applyPalette updates all active color vars from the chosen palette.
// Must be called before any widget is built (e.g. in Run) and again on toggle.
func applyPalette(dark bool) {
	p := lightPalette
	if dark {
		p = darkPalette
	}
	colorToolbar = p.toolbar
	colorSidebar = p.sidebar
	colorSurface = p.surface
	colorBorder = p.border
	colorButton = p.button
	colorText = p.text
	colorAccent = p.accent
	colorOK = p.ok
	colorWarn = p.warn
	colorErr = p.err
	colorMuted = p.muted
}

// ─── Theme ────────────────────────────────────────────────────────────────────

type flowTheme struct{}

var _ fyne.Theme = (*flowTheme)(nil)

func (flowTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colorSidebar
	case theme.ColorNameForeground:
		return colorText
	case theme.ColorNameButton:
		return colorButton
	case theme.ColorNamePrimary:
		return colorAccent
	case theme.ColorNameSeparator:
		return colorBorder
	case theme.ColorNameInputBackground:
		return colorSurface
	case theme.ColorNameInputBorder:
		return colorBorder
	case theme.ColorNamePlaceHolder:
		return colorMuted
	case theme.ColorNameScrollBar:
		return colorBorder
	case theme.ColorNameOverlayBackground:
		return colorSurface
	case theme.ColorNameMenuBackground:
		// Backs the popup of widget.Select and other menus. Without this,
		// Fyne falls back to its dark default in light mode.
		return colorSurface
	case theme.ColorNameHover:
		// Hovered list/menu item — a faint tint of the border colour reads
		// well on both light and dark surfaces.
		return colorBorder
	}
	return theme.DefaultTheme().Color(n, v)
}

func (flowTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameText {
		return 13
	}
	return theme.DefaultTheme().Size(n)
}

func (flowTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (flowTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
