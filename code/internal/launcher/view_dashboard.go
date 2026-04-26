package launcher

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showDashboardView is the per-project screen shown when a sidebar entry is
// selected. Header (name + tags + paths), status panels (working tree +
// remote), build configuration form (tool name + runners), auth widget and
// action bar.
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
				_ = u.store.RemoveProject(p.Path)
				u.current = nil
				u.refresh()
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

	// ── Build configuration ──
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
				_ = u.store.UpsertProject(p)
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
