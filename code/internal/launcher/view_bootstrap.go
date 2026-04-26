package launcher

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"flow-launcher/internal/launcher/log"
)

// showBootstrapView builds the "New project" form. The actual work runs in
// (*ui).bootstrap below, off the UI goroutine.
func (u *ui) showBootstrapView() {
	targetEntry := widget.NewEntry()
	targetEntry.SetPlaceHolder("Folder where the repository will live")

	browseBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, u.window)
				return
			}
			if uri != nil {
				targetEntry.SetText(uri.Path())
			}
		}, u.window)
	})

	remoteEntry := widget.NewEntry()
	remoteEntry.SetPlaceHolder("https://github.com/<user>/<repo>.git")

	runnerLinuxEntry := widget.NewEntry()
	runnerLinuxEntry.SetText(defaultRunnerLinux)
	runnerWindowsEntry := widget.NewEntry()
	runnerWindowsEntry.SetText(defaultRunnerWindows)

	toolNameEntry := widget.NewEntry()
	toolNameEntry.SetPlaceHolder("e.g. my-tool  (becomes the binary filename)")

	langSelect := widget.NewSelect([]string{"go", "python", "rust", "node", "java", "dotnet", "ruby", "php", "c", "cpp"}, nil)
	langSelect.SetSelected("go")

	// Auto-fill tool name when the user picks a target folder.
	targetEntry.OnChanged = func(dir string) {
		if toolNameEntry.Text == "" && dir != "" {
			toolNameEntry.SetText(sanitizeBinaryName(filepath.Base(dir)))
		}
	}

	crossInfo := muted("")
	platformRadio := widget.NewRadioGroup([]string{"cross-compiling", "both native runners", "only linux", "only windows"}, nil)
	platformRadio.Horizontal = true
	platformRadio.SetSelected("cross-compiling")

	langSelect.OnChanged = func(lang string) {
		if crossCompilingLanguages[lang] {
			crossInfo.Text = "✓  Single Linux job builds both targets via cross-compilation. Pick 'both native runners' if you need CGO + system libs (e.g. Fyne)."
			crossInfo.Color = colorOK
			platformRadio.Options = []string{"cross-compiling", "both native runners", "only linux", "only windows"}
			platformRadio.SetSelected("cross-compiling")
		} else {
			crossInfo.Text = "ℹ  This language requires native runners for each OS."
			crossInfo.Color = colorWarn
			platformRadio.Options = []string{"both native runners", "only linux", "only windows"}
			platformRadio.SetSelected("both native runners")
		}
		crossInfo.Refresh()
		platformRadio.Refresh()
	}
	langSelect.OnChanged(langSelect.Selected)

	logBox := widget.NewMultiLineEntry()
	logBox.SetMinRowsVisible(6)
	logBox.Disable()

	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	var runBtn, backBtn *widget.Button

	runBtn = widget.NewButtonWithIcon("Bootstrap project", theme.ConfirmIcon(), func() {
		dir := strings.TrimSpace(targetEntry.Text)
		remote := strings.TrimSpace(remoteEntry.Text)
		if dir == "" || remote == "" {
			dialog.ShowError(fmt.Errorf("target folder and remote URL are required"), u.window)
			return
		}
		binName := strings.TrimSpace(toolNameEntry.Text)
		if binName == "" {
			binName = sanitizeBinaryName(filepath.Base(dir))
		}
		runBtn.Disable()
		backBtn.Disable()
		progress.Show()
		go u.bootstrap(
			dir, remote,
			strings.TrimSpace(runnerLinuxEntry.Text),
			strings.TrimSpace(runnerWindowsEntry.Text),
			langSelect.Selected,
			platformValue(platformRadio.Selected),
			binName,
			logBox,
			func() {
				runOnUI(func() {
					runBtn.Enable()
					backBtn.Enable()
					progress.Hide()
				})
			},
		)
	})
	runBtn.Importance = widget.HighImportance

	backBtn = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		u.sideList.UnselectAll()
		u.showWelcomeView()
	})

	header := container.NewVBox(
		heading("New project"),
		muted("Clone the flow template, configure the workflow and register the project."),
		hSep(),
	)

	locationSection := sectionPanel("Location",
		widget.NewForm(
			widget.NewFormItem("Target folder", container.NewBorder(nil, nil, nil, browseBtn, targetEntry)),
			widget.NewFormItem("Remote URL", remoteEntry),
		))

	stackSection := sectionPanel("Stack",
		container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Tool name", toolNameEntry),
				widget.NewFormItem("Language", langSelect),
				widget.NewFormItem("Platforms", platformRadio),
			),
			crossInfo,
		))

	runnersSection := sectionPanel("Runners",
		container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Linux", runnerLinuxEntry),
				widget.NewFormItem("Windows", runnerWindowsEntry),
			),
			muted("Maps to runs-on: in the workflow. Use comma-separated tags for self-hosted sets."),
		))

	activitySection := sectionPanel("Activity", container.NewVBox(progress, logBox))

	body := container.NewVBox(locationSection, stackSection, runnersSection, activitySection)

	actionBar := container.NewBorder(nil, nil, backBtn, runBtn, widget.NewLabel(""))

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

// bootstrap runs the full setup pipeline off the UI goroutine.
func (u *ui) bootstrap(target, remote, runnerLinux, runnerWindows, lang, platform, binaryName string, logBox *widget.Entry, done func()) {
	defer done()
	logger := log.Default().With("op", "bootstrap", "target", target, "remote", remote, "lang", lang)
	logger.Info("starting bootstrap")

	appendLog := func(msg string) {
		runOnUI(func() { logBox.SetText(strings.TrimSpace(logBox.Text + "\n" + msg)) })
	}
	showErr := func(err error) {
		logger.Error("bootstrap failed", "err", err.Error())
		runOnUI(func() { dialog.ShowError(err, u.window) })
	}

	appendLog("→ Cloning template…")
	if err := cloneTemplate(u.store.TemplateURL(), target); err != nil {
		showErr(err)
		return
	}
	appendLog("✓ Template cloned")

	provider := detectProvider(remote)
	if provider == "unknown" {
		appendLog("? Could not auto-detect provider; asking…")
		picked, ok := u.askProviderBlocking(remote)
		if !ok {
			appendLog("✗ Aborted by user.")
			return
		}
		provider = picked
	}
	appendLog("✓ Provider: " + provider)

	if err := initRepository(target, remote); err != nil {
		showErr(err)
		return
	}
	appendLog("✓ Local repo initialised, origin set")

	if u.askYesNoBlocking("Sign in to "+provider+"?", "Configure a Personal Access Token now so the launcher can push to your remote without prompts.") {
		if !u.runSignInWizard(target, remote, provider, appendLog) {
			appendLog("ℹ Sign-in skipped; you may be prompted later.")
		}
	}

	appendLog("→ Checking remote reachability…")
	if err := testRemote(target); err != nil {
		if errors.Is(err, ErrAuthRequired) {
			appendLog("✗ Authentication required.")
			if !u.runSignInWizard(target, remote, provider, appendLog) {
				showErr(fmt.Errorf("aborted: cannot reach remote without credentials"))
				return
			}
			if err2 := testRemote(target); err2 != nil {
				showErr(err2)
				return
			}
		} else {
			showErr(err)
			return
		}
	}
	appendLog("✓ Remote reachable")

	if err := configureTemplate(target, provider, lang, platform, runnerLinux, runnerWindows, binaryName); err != nil {
		showErr(err)
		return
	}
	appendLog("✓ Workflow & build script configured")

	if err := cleanupTemplate(target, filepath.Base(target), binaryName); err != nil {
		showErr(err)
		return
	}
	appendLog("✓ Template cleaned up (publish.sh, build-templates/, BUILD_CONTRACT.md removed)")

	p := Project{
		Name:          filepath.Base(target),
		BinaryName:    binaryName,
		Path:          target,
		RemoteURL:     remote,
		Provider:      provider,
		Language:      lang,
		Platform:      platform,
		RunnerLinux:   runnerLinux,
		RunnerWindows: runnerWindows,
		CreatedAt:     time.Now(),
	}
	if err := u.store.UpsertProject(p); err != nil {
		showErr(err)
		return
	}
	appendLog("✓ Project saved")

	if u.askYesNoBlocking("Push initial commit?", "Push README.md as a final connectivity check.") {
		if err := commitReadme(target); err != nil {
			showErr(err)
			return
		}
		appendLog("✓ Initial commit pushed")
	} else {
		appendLog("ℹ Initial push skipped.")
	}

	u.app.SendNotification(&fyne.Notification{
		Title:   "TDAManager",
		Content: "Bootstrap completed for " + p.Name,
	})
	logger.Info("bootstrap completed", "project", p.Name)

	runOnUI(func() {
		cp := p
		u.current = &cp
		u.refresh()
		// Select the new project in the sidebar (OnSelected calls showDashboardView).
		for i, sp := range u.sideProjects {
			if sp.Path == p.Path {
				u.sideList.Select(i)
				return
			}
		}
		u.showDashboardView()
	})
}
