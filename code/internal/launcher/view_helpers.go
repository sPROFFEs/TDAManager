package launcher

import (
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

// providerColor returns the brand colour for a provider chip. Falls back to
// the muted neutral when the provider is unknown.
func providerColor(provider string) color.Color {
	switch provider {
	case "github":
		return colorGitHub
	case "gitea":
		return colorGitea
	}
	return colorMuted
}

// chip renders a small pill badge with coloured background. Used in the
// dashboard header to surface provider/language/platform at a glance.
func chip(text string, bg color.Color) fyne.CanvasObject {
	rect := canvas.NewRectangle(bg)
	rect.CornerRadius = 4
	txt := canvas.NewText(text, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	txt.TextStyle.Bold = true
	txt.TextSize = 11
	return container.NewStack(rect, container.NewPadded(container.NewCenter(txt)))
}

func heading(text string) *canvas.Text {
	t := canvas.NewText(text, colorText)
	t.TextSize = 20
	t.TextStyle.Bold = true
	return t
}

func muted(text string) *canvas.Text {
	t := canvas.NewText(text, colorMuted)
	t.TextSize = 12
	return t
}

func sectionTitle(text string) *canvas.Text {
	t := canvas.NewText(strings.ToUpper(text), colorMuted)
	t.TextStyle.Bold = true
	t.TextSize = 11
	return t
}

// hSep is a thin 1-px horizontal rule in the border colour.
func hSep() fyne.CanvasObject {
	r := canvas.NewRectangle(colorBorder)
	r.SetMinSize(fyne.NewSize(0, 1))
	return r
}

// sectionPanel composes a section title + separator + content block. Used
// in every multi-section view to keep the visual rhythm consistent.
func sectionPanel(title string, body fyne.CanvasObject) fyne.CanvasObject {
	hdr := container.NewVBox(sectionTitle(title), hSep())
	padded := container.New(layout.NewCustomPaddedLayout(6, 8, 0, 0), body)
	return container.NewVBox(hdr, padded)
}

// workflowFilePath returns the release workflow path relative to the repo root.
func workflowFilePath(provider string) string {
	if provider == "gitea" {
		return filepath.Join(".gitea", "workflows", "release.yaml")
	}
	return filepath.Join(".github", "workflows", "release.yaml")
}

// platformValue maps the radio-button label to the internal token used by
// configureWorkflow / build script editing.
func platformValue(selected string) string {
	switch selected {
	case "only linux":
		return "linux"
	case "only windows":
		return "windows"
	case "both native runners":
		return "both-native"
	default:
		return "both"
	}
}

// firstNonEmpty returns the first line of the first non-empty value among
// its arguments; if every value is blank, returns "release".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return strings.Split(v, "\n")[0]
		}
	}
	return "release"
}

// sanitizeBinaryName mirrors the shell transform used in build scripts:
// lowercase, replace anything that isn't a-z 0-9 _ - with _, trim leading
// and trailing underscores.
func sanitizeBinaryName(s string) string {
	s = strings.ToLower(s)
	var out []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}
	return strings.Trim(string(out), "_")
}

// parseInt is a digit-only ASCII int parser. Returns -1 on any non-digit.
// Used for the patch bump in the publish view.
func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}
