package launcher

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"flow-launcher/internal/launcher/log"
)

func (u *ui) showOpenView() {
	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder("Path to the existing repository folder")

	browseBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, u.window)
				return
			}
			if uri != nil {
				pathEntry.SetText(uri.Path())
			}
		}, u.window)
	})

	logBox := widget.NewMultiLineEntry()
	logBox.SetMinRowsVisible(8)
	logBox.Disable()

	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	var importBtn, backBtn *widget.Button

	importBtn = widget.NewButtonWithIcon("Import project", theme.ConfirmIcon(), func() {
		dir := strings.TrimSpace(pathEntry.Text)
		if dir == "" {
			dialog.ShowError(fmt.Errorf("select a repository folder first"), u.window)
			return
		}

		importBtn.Disable()
		backBtn.Disable()
		progress.Show()
		logBox.SetText("")

		go u.runImport(dir, logBox, func() {
			runOnUI(func() {
				importBtn.Enable()
				backBtn.Enable()
				progress.Hide()
			})
		})
	})
	importBtn.Importance = widget.HighImportance

	backBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		u.sideList.UnselectAll()
		u.showWelcomeView()
	})

	header := container.NewVBox(
		heading("Import existing project"),
		muted("Register a repository that already has the plantilla-flow workflow structure."),
		hSep(),
	)

	locationSection := sectionPanel("Repository folder",
		container.NewBorder(nil, nil, nil, browseBtn, pathEntry),
	)

	activitySection := sectionPanel("Activity", container.NewVBox(progress, logBox))

	body := container.NewVBox(locationSection, activitySection)

	actionBar := container.NewBorder(nil, nil, backBtn, importBtn, widget.NewLabel(""))

	view := container.NewStack(
		canvas.NewRectangle(colorSurface),
		container.NewBorder(
			container.New(layout.NewCustomPaddedLayout(16, 8, 16, 16), header),
			container.New(layout.NewCustomPaddedLayout(8, 12, 16, 16), actionBar),
			nil, nil,
			container.NewVScroll(
				container.New(layout.NewCustomPaddedLayout(0, 0, 16, 16), body),
			),
		),
	)
	u.setMainView(view)
}

// runImport is executed in a goroutine. It detects the repo structure, shows
// a summary in the log box, and on success saves the project and navigates to
// its dashboard. On failure it prints a diagnostic message and returns control
// to the user without crashing.
func (u *ui) runImport(dir string, logBox *widget.Entry, done func()) {
	defer done()

	logger := log.Default().With("op", "import", "dir", dir)
	logger.Info("starting import")

	appendLog := func(msg string) {
		runOnUI(func() {
			current := strings.TrimSpace(logBox.Text)
			if current == "" {
				logBox.SetText(msg)
			} else {
				logBox.SetText(current + "\n" + msg)
			}
		})
	}
	showErr := func(err error) {
		logger.Error("import failed", "err", err.Error())
		runOnUI(func() { dialog.ShowError(err, u.window) })
	}

	appendLog("→ Detecting repository structure…")

	det, err := DetectFromRepo(dir)
	if err != nil {
		if errors.Is(err, ErrNotPlantillaRepo) {
			appendLog("✗ " + err.Error())
			appendLog("")
			appendLog("This folder does not have the expected plantilla-flow structure.")
			appendLog("Create a new project with 'New project' and the launcher will set it up for you.")
		} else {
			appendLog("✗ Unexpected error: " + err.Error())
		}
		showErr(fmt.Errorf("could not import: %w", err))
		return
	}

	// ── Print detected summary ────────────────────────────────────────────
	appendLog("✓ Valid plantilla-flow repository detected")
	appendLog("")
	appendLog("  Name        " + det.Name)
	appendLog("  Provider    " + det.Provider)
	appendLog("  Language    " + det.Language)
	appendLog("  Platform    " + det.Platform)
	appendLog("  Binary      " + det.BinaryName)
	if det.RemoteURL != "" {
		appendLog("  Remote      " + det.RemoteURL)
	}
	appendLog("  Linux runner   " + det.RunnerLinux)
	if det.Language != "script" {
		appendLog("  Windows runner " + det.RunnerWindows)
	}
	if det.Language == "script" {
		appendLog("  Release mode   " + det.ReleaseMode)
	}

	if len(det.Warnings) > 0 {
		appendLog("")
		appendLog("⚠  Notes:")
		for _, w := range det.Warnings {
			appendLog("   · " + w)
		}
	}

	// ── Check for duplicate ───────────────────────────────────────────────
	if existing := u.store.FindProject(dir); existing != nil {
		appendLog("")
		appendLog("ℹ  This project is already registered — updating its metadata.")
	}

	// ── Persist ───────────────────────────────────────────────────────────
	if err := u.store.UpsertProject(det.Project); err != nil {
		showErr(fmt.Errorf("could not save project: %w", err))
		return
	}
	appendLog("")
	appendLog("✓ Project saved")
	logger.Info("import completed", "project", det.Name)

	u.app.SendNotification(&fyne.Notification{
		Title:   "TDAManager",
		Content: "Project '" + det.Name + "' imported successfully.",
	})

	runOnUI(func() {
		cp := det.Project
		u.current = &cp
		u.refresh()
		for i, sp := range u.sideProjects {
			if sp.Path == det.Path {
				u.sideList.Select(i)
				return
			}
		}
		u.showDashboardView()
	})
}
