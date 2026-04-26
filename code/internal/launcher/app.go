package launcher

import (
	"fmt"
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"flow-launcher/internal/launcher/log"
)

// ─── Application shell ────────────────────────────────────────────────────────
//
// ui owns the top-level chrome (toolbar + sidebar) and the swap-target main
// view. View bodies live in their own files (view_*.go) so this file stays
// small and focused on navigation plumbing.

type ui struct {
	app    fyne.App
	window fyne.Window
	store  *Store
	state  AppState // cached snapshot of store; refreshed via u.refresh()

	current      *Project
	mainView     *fyne.Container // swapped on each navigation
	sideList     *widget.List
	sideProjects []Project // sorted slice backing sideList
}

func Run() {
	logger := log.Init()
	defer log.Close()
	logger.Info("launcher starting", "appID", appID)

	a := app.NewWithID(appID)
	store := Open()
	applyPalette(store.DarkMode())
	a.Settings().SetTheme(&flowTheme{})

	u := &ui{
		app:    a,
		window: a.NewWindow("TDAManager - Tool Deployment Automated Manager"),
		store:  store,
		state:  store.Snapshot(),
	}
	u.sideProjects = store.Projects()
	u.mainView = container.NewStack()

	u.window.SetContent(u.buildChrome())
	u.showWelcomeView()
	u.window.Resize(fyne.NewSize(1140, 780))
	u.window.ShowAndRun()
	logger.Info("launcher exiting")
}

// refresh pulls a fresh snapshot from the store and re-syncs the sidebar.
// Call after any mutation through the store.
func (u *ui) refresh() {
	u.state = u.store.Snapshot()
	u.sideProjects = u.store.Projects()
	if u.sideList != nil {
		u.sideList.Refresh()
	}
}

// toggleDarkMode flips the colour scheme, persists the choice, and rebuilds the
// entire window so every canvas object picks up the new palette colours.
func (u *ui) toggleDarkMode() {
	if err := u.store.SetDarkMode(!u.store.DarkMode()); err != nil {
		log.Default().Error("toggle dark mode failed", "err", err.Error())
	}
	u.state = u.store.Snapshot()
	applyPalette(u.state.DarkMode)
	u.app.Settings().SetTheme(&flowTheme{})

	u.sideList = nil
	u.sideProjects = u.store.Projects()
	u.mainView = container.NewStack()
	u.window.SetContent(u.buildChrome())

	if u.current != nil {
		u.showDashboardView()
	} else {
		u.showWelcomeView()
	}
}

// runOnUI is a thin indirection for UI mutations from goroutines. Fyne v2.5
// widget Set/Refresh methods are goroutine-safe; replace with fyne.Do on v2.6+.
func runOnUI(f func()) { f() }

// ─── Chrome (persistent shell) ────────────────────────────────────────────────

func (u *ui) buildChrome() fyne.CanvasObject {
	split := container.NewHSplit(u.buildSidebar(), u.mainView)
	split.Offset = 0.26
	return container.NewBorder(u.buildToolbar(), nil, nil, nil, split)
}

func (u *ui) buildToolbar() fyne.CanvasObject {
	// Two-tone logo: "TDA" in accent blue, "Manager" in the toolbar-contrast colour.
	// canvas.Text uses the color captured at construction time; since buildToolbar
	// is called after applyPalette on every toggle, the colours are always current.
	// We avoid emoji here because Windows canvas.Text renders them unreliably.
	toolbarText := color.NRGBA{R: 255, G: 255, B: 255, A: 255} // white on dark toolbar
	if !u.state.DarkMode {
		toolbarText = colorText // dark text on light toolbar
	}
	logoTDA := canvas.NewText("TDA", colorAccent)
	logoTDA.TextStyle.Bold = true
	logoTDA.TextSize = 15
	logoManager := canvas.NewText("Manager", toolbarText)
	logoManager.TextStyle.Bold = true
	logoManager.TextSize = 15
	logo := container.NewHBox(logoTDA, logoManager)

	modeIcon := fyne.Resource(moonIcon)
	modeLabel := "Dark"
	if u.state.DarkMode {
		modeIcon = sunIcon
		modeLabel = "Light"
	}
	modeBtn := widget.NewButtonWithIcon(modeLabel, modeIcon, u.toggleDarkMode)
	modeBtn.Importance = widget.HighImportance

	settingsBtn := widget.NewButtonWithIcon("Template source", theme.SettingsIcon(), u.showTemplateSettings)
	settingsBtn.Importance = widget.HighImportance

	row := container.NewBorder(nil, nil, logo, container.NewHBox(modeBtn, settingsBtn), nil)
	bg := canvas.NewRectangle(colorToolbar)
	sep := canvas.NewRectangle(colorBorder)
	sep.SetMinSize(fyne.NewSize(0, 1))
	inner := container.New(layout.NewCustomPaddedLayout(11, 11, 14, 14), row)
	return container.NewBorder(nil, sep, nil, nil, container.NewStack(bg, inner))
}

func (u *ui) buildSidebar() fyne.CanvasObject {
	header := container.New(
		layout.NewCustomPaddedLayout(10, 8, 12, 12),
		container.NewBorder(nil, nil,
			sectionTitle("Projects"),
			canvas.NewText(fmt.Sprintf("(%d)", len(u.sideProjects)), colorMuted),
			nil,
		),
	)
	headerBg := canvas.NewRectangle(colorSidebar)
	styledHeader := container.NewStack(headerBg, header)

	u.sideList = widget.NewList(
		func() int { return len(u.sideProjects) },

		// Template: border(center=paddedVBox, left=colorBar)
		func() fyne.CanvasObject {
			bar := canvas.NewRectangle(colorMuted)
			bar.SetMinSize(fyne.NewSize(3, 50))
			name := widget.NewLabel("Project name")
			name.TextStyle.Bold = true
			meta := widget.NewLabel("provider · lang · platform")
			inner := container.NewVBox(name, meta)
			padded := container.New(layout.NewCustomPaddedLayout(7, 7, 10, 8), inner)
			return container.NewBorder(nil, nil, bar, nil, padded)
		},

		// Update: Objects=[padded(0), bar(1)], padded.Objects=[vbox(0)]
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			p := u.sideProjects[id]
			row := obj.(*fyne.Container)
			padded := row.Objects[0].(*fyne.Container)
			vbox := padded.Objects[0].(*fyne.Container)
			nameLabel := vbox.Objects[0].(*widget.Label)
			metaLabel := vbox.Objects[1].(*widget.Label)
			bar := row.Objects[1].(*canvas.Rectangle)

			bar.FillColor = providerColor(p.Provider)
			bar.Refresh()

			marker := ""
			if _, err := os.Stat(p.Path); err != nil {
				marker = " ⚠"
			}
			nameLabel.SetText(p.Name + marker)
			metaLabel.SetText(p.Provider + " · " + p.Language + " · " + p.Platform)
		},
	)

	u.sideList.OnSelected = func(id widget.ListItemID) {
		p := u.sideProjects[id]
		if _, err := os.Stat(p.Path); err != nil {
			dialog.ShowConfirm(
				"Project folder missing",
				fmt.Sprintf("The folder for '%s' no longer exists.\n%s\n\nRemove it from the list?", p.Name, p.Path),
				func(ok bool) {
					if ok {
						_ = u.store.RemoveProject(p.Path)
						u.refresh()
						u.showWelcomeView()
					} else {
						u.sideList.UnselectAll()
					}
				}, u.window)
			return
		}
		_ = u.store.TouchProject(p.Path)
		u.state = u.store.Snapshot()
		cp := p
		u.current = &cp
		u.showDashboardView()
	}

	newBtn := widget.NewButtonWithIcon("New project", theme.ContentAddIcon(), func() {
		u.sideList.UnselectAll()
		u.showBootstrapView()
	})
	newBtn.Importance = widget.HighImportance
	newBtnRow := container.New(layout.NewCustomPaddedLayout(8, 8, 10, 10), newBtn)

	sep := canvas.NewRectangle(colorBorder)
	sep.SetMinSize(fyne.NewSize(0, 1))

	bg := canvas.NewRectangle(colorSidebar)
	content := container.NewBorder(
		container.NewVBox(styledHeader, sep),
		container.NewVBox(sep, newBtnRow),
		nil, nil,
		u.sideList,
	)
	return container.NewStack(bg, content)
}

func (u *ui) setMainView(content fyne.CanvasObject) {
	u.mainView.Objects = []fyne.CanvasObject{content}
	u.mainView.Refresh()
}

// refreshSidebar is kept as an alias for u.refresh() so view files that
// only need the sidebar update can use a clear name.
func (u *ui) refreshSidebar() { u.refresh() }
