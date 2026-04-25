package launcher

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ─── Application shell ────────────────────────────────────────────────────────

type ui struct {
	app          fyne.App
	window       fyne.Window
	state        AppState
	current      *Project
	mainView     *fyne.Container // swapped on each navigation
	sideList     *widget.List
	sideProjects []Project // sorted slice backing sideList
}

func Run() {
	a := app.NewWithID(appID)
	state := loadState()
	applyPalette(state.DarkMode)
	a.Settings().SetTheme(&flowTheme{})

	u := &ui{
		app:    a,
		window: a.NewWindow("TDAManager - Tool Deployment Automated Manager"),
		state:  state,
	}
	u.sideProjects = projectsSortedByRecent(u.state)
	u.mainView = container.NewStack()

	u.window.SetContent(u.buildChrome())
	u.showWelcomeView()
	u.window.Resize(fyne.NewSize(1140, 780))
	u.window.ShowAndRun()
}

// toggleDarkMode flips the colour scheme, persists the choice, and rebuilds the
// entire window so every canvas object picks up the new palette colours.
func (u *ui) toggleDarkMode() {
	u.state.DarkMode = !u.state.DarkMode
	_ = saveState(u.state)
	applyPalette(u.state.DarkMode)
	u.app.Settings().SetTheme(&flowTheme{})

	u.sideList = nil
	u.sideProjects = projectsSortedByRecent(u.state)
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
						removeProject(&u.state, p.Path)
						_ = saveState(u.state)
						u.refreshSidebar()
						u.showWelcomeView()
					} else {
						u.sideList.UnselectAll()
					}
				}, u.window)
			return
		}
		touchProject(&u.state, p.Path)
		_ = saveState(u.state)
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

func (u *ui) refreshSidebar() {
	u.sideProjects = projectsSortedByRecent(u.state)
	if u.sideList != nil {
		u.sideList.Refresh()
	}
}

// ─── Welcome view ─────────────────────────────────────────────────────────────

func (u *ui) showWelcomeView() {
	u.current = nil

	var body fyne.CanvasObject
	if len(u.sideProjects) == 0 {
		title := canvas.NewText("No projects yet", colorMuted)
		title.TextSize = 18
		sub := muted("Create your first project to get started.")
		btn := widget.NewButtonWithIcon("New project", theme.ContentAddIcon(), func() {
			u.sideList.UnselectAll()
			u.showBootstrapView()
		})
		btn.Importance = widget.HighImportance
		body = container.NewCenter(container.NewVBox(
			container.NewCenter(title),
			container.NewCenter(sub),
			widget.NewLabel(""),
			container.NewCenter(btn),
		))
	} else {
		title := canvas.NewText("Select a project", colorMuted)
		title.TextSize = 18
		sub := muted("Choose a project from the sidebar, or create a new one.")
		body = container.NewCenter(container.NewVBox(
			container.NewCenter(title),
			container.NewCenter(sub),
		))
	}

	u.setMainView(container.NewStack(canvas.NewRectangle(colorSurface), body))
}

// ─── Bootstrap view ───────────────────────────────────────────────────────────

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

	appendLog := func(msg string) {
		runOnUI(func() { logBox.SetText(strings.TrimSpace(logBox.Text + "\n" + msg)) })
	}
	showErr := func(err error) {
		runOnUI(func() { dialog.ShowError(err, u.window) })
	}

	appendLog("→ Cloning template…")
	if err := cloneTemplate(u.state.TemplateURL, target); err != nil {
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
	upsertProject(&u.state, p)
	if err := saveState(u.state); err != nil {
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

	runOnUI(func() {
		cp := p
		u.current = &cp
		u.refreshSidebar()
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

// ─── Dashboard view ───────────────────────────────────────────────────────────

func (u *ui) showDashboardView() {
	if u.current == nil {
		u.showWelcomeView()
		return
	}
	p := *u.current

	titleText := heading(p.Name)
	tags := container.NewHBox(
		chip(p.Provider, providerColor(p.Provider)),
		chip(p.Language, colorAccent),
		chip(p.Platform, colorMuted),
	)
	pathText := muted(p.Path)
	remoteText := muted(p.RemoteURL)

	removeBtn := widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
		dialog.ShowConfirm(
			"Remove project",
			fmt.Sprintf("Remove '%s' from TDAManager?\n\nThis only removes it from the launcher — local files are NOT deleted.", p.Name),
			func(ok bool) {
				if !ok {
					return
				}
				removeProject(&u.state, p.Path)
				_ = saveState(u.state)
				u.current = nil
				u.refreshSidebar()
				u.showWelcomeView()
			}, u.window)
	})
	removeBtn.Importance = widget.DangerImportance

	headerContent := container.NewVBox(
		container.NewBorder(nil, nil, titleText, removeBtn, nil),
		tags,
		pathText,
		remoteText,
	)

	// ── Status panels ──
	statusBox := widget.NewMultiLineEntry()
	statusBox.SetMinRowsVisible(6)
	statusBox.Disable()

	remoteLine := canvas.NewText("Checking remote…", colorMuted)
	remoteLine.TextStyle.Bold = true
	remoteLine.TextSize = 13

	loadStatus := func() {
		go func() {
			gs := gitStatus(p.Path)
			rs := remoteStatus(p.Path)
			runOnUI(func() {
				statusBox.SetText(gs)
				if strings.HasPrefix(rs, "Remote reachable") {
					remoteLine.Text = "✓  Remote reachable"
					remoteLine.Color = colorOK
				} else {
					remoteLine.Text = "✗  Remote unavailable"
					remoteLine.Color = colorErr
					// Append the git detail to the working-tree box so the user
					// can read it without it breaking the layout.
					if detail := strings.TrimPrefix(rs, "Remote unavailable:\n"); detail != "" {
						statusBox.SetText(strings.TrimSpace(gs + "\n\n─── Remote ───\n" + detail))
					}
				}
				remoteLine.Refresh()
			})
		}()
	}
	loadStatus()

	remoteContent := container.NewVBox(remoteLine, muted(p.RemoteURL))
	statusSection := sectionPanel("Working tree", statusBox)
	remoteSection := sectionPanel("Remote", remoteContent)
	statusRow := container.New(layout.NewGridLayoutWithColumns(2), remoteSection, statusSection)

	// ── Runners & build config ──
	binName := p.BinaryName
	if binName == "" {
		binName = sanitizeBinaryName(p.Name)
	}
	binaryNameEntry := widget.NewEntry()
	binaryNameEntry.SetText(binName)

	runnerLinuxEntry := widget.NewEntry()
	runnerLinuxEntry.SetText(p.RunnerLinux)
	runnerWindowsEntry := widget.NewEntry()
	runnerWindowsEntry.SetText(p.RunnerWindows)

	saveRunnersBtn := widget.NewButtonWithIcon("Save & sync", theme.DocumentSaveIcon(), nil)
	saveRunnersBtn.OnTapped = func() {
		nl := strings.TrimSpace(runnerLinuxEntry.Text)
		nw := strings.TrimSpace(runnerWindowsEntry.Text)
		nb := strings.TrimSpace(binaryNameEntry.Text)
		if nl == "" || nw == "" {
			dialog.ShowError(errors.New("both runner fields are required"), u.window)
			return
		}
		if nb == "" {
			dialog.ShowError(errors.New("tool name cannot be empty"), u.window)
			return
		}
		saveRunnersBtn.Disable()
		go func() {
			defer runOnUI(func() { saveRunnersBtn.Enable() })
			if err := UpdateRunners(p.Path, p.Provider, nl, nw); err != nil {
				runOnUI(func() { dialog.ShowError(err, u.window) })
				return
			}
			if nb != p.BinaryName {
				if err := UpdateBinaryName(p.Path, nb); err != nil {
					runOnUI(func() { dialog.ShowError(err, u.window) })
					return
				}
			}
			rel := workflowFilePath(p.Provider)
			if res := runGit(p.Path, "add", rel, "build.sh"); res.Err != nil {
				runOnUI(func() { dialog.ShowError(errors.New("git add: "+res.Output), u.window) })
				return
			}
			if res := runGit(p.Path, "commit", "-m", "Update runner tags via TDAManager"); res.Err != nil && !strings.Contains(res.Output, "nothing to commit") {
				runOnUI(func() { dialog.ShowError(errors.New("git commit: "+res.Output), u.window) })
				return
			}
			if res := runGit(p.Path, "push"); res.Err != nil {
				if errors.Is(classifyGitError(res.Output), ErrAuthRequired) {
					if !u.runSignInWizard(p.Path, p.RemoteURL, p.Provider, func(string) {}) {
						runOnUI(func() { dialog.ShowError(errors.New("push aborted: missing credentials"), u.window) })
						return
					}
					if res2 := runGit(p.Path, "push"); res2.Err != nil {
						runOnUI(func() { dialog.ShowError(errors.New("git push: "+res2.Output), u.window) })
						return
					}
				} else {
					runOnUI(func() { dialog.ShowError(errors.New("git push: "+res.Output), u.window) })
					return
				}
			}
			runOnUI(func() {
				p.RunnerLinux = nl
				p.RunnerWindows = nw
				p.BinaryName = nb
				upsertProject(&u.state, p)
				_ = saveState(u.state)
				u.current = &p
				dialog.ShowInformation("Configuration updated", "Changes committed and pushed.", u.window)
				loadStatus()
			})
		}()
	}

	runnersSection := sectionPanel("Build configuration",
		container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Tool name", binaryNameEntry),
				widget.NewFormItem("Linux runner", runnerLinuxEntry),
				widget.NewFormItem("Windows runner", runnerWindowsEntry),
			),
			container.NewBorder(nil, nil, nil, saveRunnersBtn, widget.NewLabel("")),
		))

	// ── Auth state ──
	authStatus := canvas.NewText("Checking…", colorMuted)
	authStatus.TextSize = 12
	authBtn := widget.NewButtonWithIcon("Sign in", theme.AccountIcon(), nil)

	var setAuthState func(bool)
	setAuthState = func(ok bool) {
		if ok {
			authStatus.Text = "✓  Authenticated"
			authStatus.Color = colorOK
			authBtn.SetText("Logout")
			authBtn.Icon = theme.CancelIcon()
			authBtn.Importance = widget.LowImportance
			authBtn.OnTapped = func() {
				dialog.ShowConfirm("Logout",
					"Remove saved credentials for "+p.Provider+"?\nYou will need to sign in again to push.",
					func(confirmed bool) {
						if !confirmed {
							return
						}
						go func() {
							if err := RevokeAuth(p.Path); err != nil {
								runOnUI(func() { dialog.ShowError(err, u.window) })
								return
							}
							runOnUI(func() {
								setAuthState(false)
								loadStatus()
							})
						}()
					}, u.window)
			}
		} else {
			authStatus.Text = "Not logged in"
			authStatus.Color = colorMuted
			authBtn.SetText("Sign in")
			authBtn.Icon = theme.AccountIcon()
			authBtn.Importance = widget.MediumImportance
			authBtn.OnTapped = func() {
				go func() {
					if u.runSignInWizard(p.Path, p.RemoteURL, p.Provider, func(string) {}) {
						runOnUI(func() {
							setAuthState(true)
							loadStatus()
						})
					}
				}()
			}
		}
		authStatus.Refresh()
		authBtn.Refresh()
	}

	checkAuth := func() {
		// Give instant feedback from the local credential file before hitting the network.
		if IsAuthConfigured(p.Path) {
			runOnUI(func() {
				authStatus.Text = "Verifying…"
				authStatus.Color = colorMuted
				authStatus.Refresh()
			})
		}
		ok := VerifyRemoteAuth(p.Path) == nil
		runOnUI(func() { setAuthState(ok) })
	}
	go checkAuth()

	// ── Action bar ──
	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		loadStatus()
		go checkAuth()
	})
	openRemoteBtn := widget.NewButtonWithIcon("Open remote", theme.SearchIcon(), func() {
		_ = openBrowser(p.RemoteURL)
	})
	publishBtn := widget.NewButtonWithIcon("Publish release", theme.UploadIcon(), func() {
		u.showPublishView(p)
	})
	publishBtn.Importance = widget.HighImportance

	actionBar := container.NewBorder(nil, nil,
		container.NewHBox(authBtn, authStatus, refreshBtn, openRemoteBtn),
		publishBtn,
		widget.NewLabel(""),
	)

	body := container.NewVBox(
		headerContent,
		hSep(),
		statusRow,
		runnersSection,
	)

	view := container.NewStack(
		canvas.NewRectangle(colorSurface),
		container.NewBorder(
			nil,
			container.New(layout.NewCustomPaddedLayout(8, 12, 16, 16), actionBar),
			nil, nil,
			container.NewVScroll(
				container.New(layout.NewCustomPaddedLayout(16, 0, 16, 16), body),
			),
		),
	)
	u.setMainView(view)
}

// ─── Publish view ─────────────────────────────────────────────────────────────

func (u *ui) showPublishView(p Project) {
	current := readLatestVersion(p.Path)

	// Pre-fill with a patch bump of the current version, or 1.0.0 for a fresh repo.
	maj, min, pat := "1", "0", "0"
	if current != "none" {
		parts := strings.SplitN(current, ".", 3)
		if len(parts) == 3 {
			maj, min, pat = parts[0], parts[1], parts[2]
			// Default suggestion: bump patch.
			if n := parseInt(pat); n >= 0 {
				pat = fmt.Sprintf("%d", n+1)
			}
		}
	}

	major := widget.NewEntry()
	minor := widget.NewEntry()
	patch := widget.NewEntry()
	major.SetText(maj)
	minor.SetText(min)
	patch.SetText(pat)

	added := widget.NewMultiLineEntry()
	changed := widget.NewMultiLineEntry()
	fixed := widget.NewMultiLineEntry()

	versionBox := container.NewHBox(
		widget.NewLabelWithStyle("v", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		major, widget.NewLabel("."), minor, widget.NewLabel("."), patch,
	)

	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	currentStr := "v" + current
	if current == "none" {
		currentStr = "none (first release)"
	}
	currentVersionLabel := canvas.NewText("Current: "+currentStr, colorMuted)
	currentVersionLabel.TextSize = 12

	versionSection := sectionPanel("Version",
		container.NewVBox(
			currentVersionLabel,
			widget.NewForm(widget.NewFormItem("New tag", versionBox)),
		))

	notesSection := sectionPanel("Release notes",
		widget.NewForm(
			widget.NewFormItem("Added", added),
			widget.NewFormItem("Changed", changed),
			widget.NewFormItem("Fixed", fixed),
		))

	var confirmBtn, backBtn *widget.Button

	confirmBtn = widget.NewButtonWithIcon("Commit, tag and push", theme.ConfirmIcon(), func() {
		version := fmt.Sprintf("v%s.%s.%s",
			strings.TrimSpace(major.Text),
			strings.TrimSpace(minor.Text),
			strings.TrimSpace(patch.Text),
		)
		if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
			dialog.ShowError(fmt.Errorf("version must match vX.Y.Z (numbers only)"), u.window)
			return
		}
		msg := firstNonEmpty(added.Text, changed.Text, fixed.Text, "release")

		dialog.ShowConfirm("Publish release",
			"Update CHANGELOG.md, commit, push main, create tag "+version+" and push tag?",
			func(ok bool) {
				if !ok {
					return
				}
				confirmBtn.Disable()
				backBtn.Disable()
				progress.Show()
				go func() {
					defer runOnUI(func() {
						confirmBtn.Enable()
						backBtn.Enable()
						progress.Hide()
					})
					if err := injectChangelog(p.Path, version, added.Text, changed.Text, fixed.Text); err != nil {
						runOnUI(func() { dialog.ShowError(err, u.window) })
						return
					}
					runPublish := func() error { return publish(p.Path, version, msg) }
					if err := runPublish(); err != nil {
						if errors.Is(err, ErrAuthRequired) {
							if !u.runSignInWizard(p.Path, p.RemoteURL, p.Provider, func(string) {}) {
								runOnUI(func() { dialog.ShowError(errors.New("publish aborted: missing credentials"), u.window) })
								return
							}
							if err2 := runPublish(); err2 != nil {
								runOnUI(func() { dialog.ShowError(err2, u.window) })
								return
							}
						} else {
							runOnUI(func() { dialog.ShowError(err, u.window) })
							return
						}
					}
					runOnUI(func() {
						dialog.ShowInformation("Published", "Release pushed. Review Actions on the remote repository.", u.window)
						u.showDashboardView()
					})
				}()
			}, u.window)
	})
	confirmBtn.Importance = widget.HighImportance
	backBtn = widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), u.showDashboardView)

	header := container.NewVBox(
		heading("Publish release"),
		muted("Tag format: vX.Y.Z · CHANGELOG and tag are pushed atomically."),
		hSep(),
	)
	body := container.NewVBox(versionSection, notesSection, progress)
	actionBar := container.NewBorder(nil, nil, backBtn, confirmBtn, widget.NewLabel(""))

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

// ─── Template settings ────────────────────────────────────────────────────────

func (u *ui) showTemplateSettings() {
	entry := widget.NewEntry()
	entry.SetText(u.state.TemplateURL)
	form := dialog.NewForm("Template repository", "Save", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Repository URL", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			u.state.TemplateURL = strings.TrimSpace(entry.Text)
			_ = saveState(u.state)
		}, u.window)
	form.Resize(fyne.NewSize(620, 140))
	form.Show()
}

// ─── Blocking dialogs (called from goroutines) ────────────────────────────────

func (u *ui) askProviderBlocking(remote string) (string, bool) {
	type result struct {
		provider string
		ok       bool
	}
	ch := make(chan result, 1)
	runOnUI(func() {
		sel := widget.NewRadioGroup([]string{"github", "gitea"}, nil)
		sel.SetSelected("github")
		hint := widget.NewLabel("Auto-detection failed for: " + remote + "\nPick the provider so the right workflow folder is kept.")
		hint.Wrapping = fyne.TextWrapWord
		content := container.NewVBox(hint, sel)
		dlg := dialog.NewCustomConfirm("Select provider", "Continue", "Cancel", content, func(ok bool) {
			if !ok || sel.Selected == "" {
				ch <- result{ok: false}
				return
			}
			ch <- result{provider: sel.Selected, ok: true}
		}, u.window)
		dlg.Resize(fyne.NewSize(520, 200))
		dlg.Show()
	})
	r := <-ch
	return r.provider, r.ok
}

func (u *ui) askYesNoBlocking(title, body string) bool {
	ch := make(chan bool, 1)
	runOnUI(func() {
		dialog.ShowConfirm(title, body, func(ok bool) { ch <- ok }, u.window)
	})
	return <-ch
}

func (u *ui) runSignInWizard(projectDir, remoteURL, provider string, log func(string)) bool {
	type result struct{ ok bool }
	ch := make(chan result, 1)

	runOnUI(func() {
		usernameEntry := widget.NewEntry()
		usernameEntry.SetText("oauth2")
		usernameEntry.SetPlaceHolder("Username (use 'oauth2' for PATs)")

		tokenEntry := widget.NewPasswordEntry()
		tokenEntry.SetPlaceHolder("Personal Access Token")

		statusLine := canvas.NewText("", colorMuted)
		statusLine.TextSize = 12

		setStatus := func(text string, c color.Color) {
			statusLine.Text = text
			statusLine.Color = c
			statusLine.Refresh()
		}

		tokenURL := TokenCreationURL(provider, remoteURL)
		intro := widget.NewLabel(
			"Sign in to " + provider + " so the launcher can push to your remote.\n\n" +
				"1.  Open the token page below.\n" +
				"2.  Create a token with 'repo' scope (and 'workflow' on GitHub).\n" +
				"3.  Paste it here and click Verify.",
		)
		intro.Wrapping = fyne.TextWrapWord

		openBtn := widget.NewButtonWithIcon("Open token page", theme.SearchIcon(), func() {
			if tokenURL == "" {
				setStatus("Token URL is unknown for this provider.", colorWarn)
				return
			}
			_ = openBrowser(tokenURL)
		})

		verifyBtn := widget.NewButtonWithIcon("Verify & save", theme.ConfirmIcon(), nil)
		cancelBtn := widget.NewButtonWithIcon("Skip", theme.CancelIcon(), nil)

		var dlg dialog.Dialog
		verifyBtn.OnTapped = func() {
			user := strings.TrimSpace(usernameEntry.Text)
			tok := strings.TrimSpace(tokenEntry.Text)
			if tok == "" {
				setStatus("Token cannot be empty.", colorErr)
				return
			}
			verifyBtn.Disable()
			cancelBtn.Disable()
			setStatus("Verifying…", colorAccent)
			go func() {
				if err := SetRemoteAuth(projectDir, remoteURL, user, tok); err != nil {
					runOnUI(func() {
						setStatus("Could not save credentials: "+err.Error(), colorErr)
						verifyBtn.Enable()
						cancelBtn.Enable()
					})
					return
				}
				if err := VerifyRemoteAuth(projectDir); err != nil {
					runOnUI(func() {
						setStatus("Authentication failed against the remote.", colorErr)
						verifyBtn.Enable()
						cancelBtn.Enable()
					})
					log("✗ Token rejected by remote")
					return
				}
				runOnUI(func() {
					setStatus("✓ Authenticated.", colorOK)
					log("✓ Token verified and stored")
					ch <- result{ok: true}
					dlg.Hide()
				})
			}()
		}
		cancelBtn.OnTapped = func() {
			ch <- result{ok: false}
			dlg.Hide()
		}

		body := container.NewVBox(
			intro,
			widget.NewForm(
				widget.NewFormItem("Username", usernameEntry),
				widget.NewFormItem("Token", tokenEntry),
			),
			container.NewHBox(openBtn, layout.NewSpacer(), cancelBtn, verifyBtn),
			statusLine,
		)
		dlg = dialog.NewCustomWithoutButtons("Sign in to "+provider, body, u.window)
		dlg.Resize(fyne.NewSize(640, 360))
		dlg.Show()
	})

	r := <-ch
	return r.ok
}

// ─── Visual helpers ───────────────────────────────────────────────────────────

func providerColor(provider string) color.Color {
	switch provider {
	case "github":
		return colorGitHub
	case "gitea":
		return colorGitea
	}
	return colorMuted
}

// chip renders a small pill badge with colored background.
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

// sectionPanel composes a section title + separator + content block.
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

// ─── Utilities ────────────────────────────────────────────────────────────────

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
// lowercase, replace anything that isn't a-z 0-9 _ - with _.
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
