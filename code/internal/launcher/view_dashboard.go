package launcher

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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
	isScript := p.Language == "script"

	artifactName := p.BinaryName
	if artifactName == "" {
		artifactName = sanitizeBinaryName(p.Name)
	}
	artifactNameEntry := widget.NewEntry()
	artifactNameEntry.SetText(artifactName)

	runnerLinuxEntry := widget.NewEntry()
	runnerLinuxEntry.SetText(p.RunnerLinux)
	runnerWindowsEntry := widget.NewEntry()
	runnerWindowsEntry.SetText(p.RunnerWindows)

	saveConfigBtn := widget.NewButtonWithIcon("Save & sync", theme.DocumentSaveIcon(), nil)

	var configSection fyne.CanvasObject

	if isScript {
		// ── Script project: release mode + file list or archive name ────────
		releaseMode := p.ReleaseMode
		if releaseMode == "" {
			releaseMode = "archive"
		}

		// Dynamic file list for "specific files" mode.
		fileList := strings.Fields(p.ReleaseFiles)

		filesBox := container.NewVBox()
		var rebuildFileList func()
		rebuildFileList = func() {
			filesBox.Objects = nil
			if len(fileList) == 0 {
				filesBox.Add(muted("No files added yet — use + to select files from the code/ directory."))
			} else {
				for i := range fileList {
					i := i
					row := container.NewBorder(nil, nil, nil,
						widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
							fileList = append(fileList[:i], fileList[i+1:]...)
							rebuildFileList()
						}),
						widget.NewLabel(fileList[i]),
					)
					filesBox.Add(row)
				}
			}
			filesBox.Refresh()
		}
		rebuildFileList()

		addFileBtn := widget.NewButtonWithIcon("Add file", theme.ContentAddIcon(), func() {
			codeDir := filepath.Join(p.Path, "code")
			fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
				if rc == nil || err != nil {
					return
				}
				defer rc.Close()
				selected := rc.URI().Path()
				rel, relErr := filepath.Rel(codeDir, selected)
				if relErr != nil || strings.HasPrefix(rel, "..") {
					runOnUI(func() {
						dialog.ShowError(errors.New("selected file must be inside the code/ directory"), u.window)
					})
					return
				}
				rel = filepath.ToSlash(rel)
				fileList = append(fileList, rel)
				runOnUI(rebuildFileList)
			}, u.window)
			if luri, lErr := storage.ListerForURI(storage.NewFileURI(codeDir)); lErr == nil {
				fd.SetLocation(luri)
			}
			fd.Show()
		})

		filesSection := container.NewVBox(filesBox, addFileBtn)

		// Archive name row (shown only in archive mode).
		archiveLabel := widget.NewLabel("Archive name")
		archiveNameRow := container.New(layout.NewFormLayout(), archiveLabel, artifactNameEntry)

		// Hint shown in files mode.
		archiveHint := muted("Each file keeps its own filename in the release.")

		if releaseMode == "files" {
			archiveNameRow.Hide()
			archiveHint.Show()
		} else {
			filesSection.Hide()
			archiveHint.Hide()
		}

		releaseModeSelect := widget.NewSelect([]string{"archive code/ folder", "specific files"}, nil)
		if releaseMode == "files" {
			releaseModeSelect.SetSelected("specific files")
		} else {
			releaseModeSelect.SetSelected("archive code/ folder")
		}
		releaseModeSelect.OnChanged = func(selected string) {
			if selected == "specific files" {
				archiveNameRow.Hide()
				archiveHint.Show()
				filesSection.Show()
			} else {
				archiveNameRow.Show()
				archiveHint.Hide()
				filesSection.Hide()
			}
		}

		saveConfigBtn.OnTapped = func() {
			nl := strings.TrimSpace(runnerLinuxEntry.Text)
			if nl == "" {
				dialog.ShowError(errors.New("Linux runner field is required"), u.window)
				return
			}
			nm := "archive"
			nf := ""
			na := strings.TrimSpace(artifactNameEntry.Text)
			if releaseModeSelect.Selected == "specific files" {
				nm = "files"
				nf = strings.Join(fileList, " ")
				if nf == "" {
					dialog.ShowError(errors.New("add at least one file before saving"), u.window)
					return
				}
				// Use project name as APP_NAME when not archiving.
				if na == "" {
					na = sanitizeBinaryName(p.Name)
				}
			} else {
				if na == "" {
					dialog.ShowError(errors.New("archive name cannot be empty"), u.window)
					return
				}
			}
			saveConfigBtn.Disable()
			go func() {
				defer runOnUI(func() { saveConfigBtn.Enable() })
				if err := UpdateRunners(p.Path, p.Provider, nl, ""); err != nil {
					runOnUI(func() { dialog.ShowError(err, u.window) })
					return
				}
				if err := UpdateReleaseConfig(p.Path, na, nm, nf); err != nil {
					runOnUI(func() { dialog.ShowError(err, u.window) })
					return
				}
				rel := workflowFilePath(p.Provider)
				if res := runGit(p.Path, "add", rel, "build.sh"); res.Err != nil {
					runOnUI(func() { dialog.ShowError(errors.New("git add: "+res.Output), u.window) })
					return
				}
				if res := runGit(p.Path, "commit", "-m", "Update release configuration via TDAManager"); res.Err != nil && !strings.Contains(res.Output, "nothing to commit") {
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
					p.BinaryName = na
					p.ReleaseMode = nm
					p.ReleaseFiles = nf
					_ = u.store.UpsertProject(p)
					u.current = &p
					dialog.ShowInformation("Configuration updated", "Changes committed and pushed.", u.window)
					loadStatus()
				})
			}()
		}

		configSection = sectionPanel("Release configuration",
			container.NewVBox(
				container.New(layout.NewFormLayout(),
					widget.NewLabel("Mode"), releaseModeSelect,
					widget.NewLabel("Linux runner"), runnerLinuxEntry,
				),
				archiveNameRow,
				archiveHint,
				filesSection,
				container.NewBorder(nil, nil, nil, saveConfigBtn, widget.NewLabel("")),
			))

	} else {
		// ── Compiled project: binary name + both runners ─────────────────────
		saveConfigBtn.OnTapped = func() {
			nl := strings.TrimSpace(runnerLinuxEntry.Text)
			nw := strings.TrimSpace(runnerWindowsEntry.Text)
			nb := strings.TrimSpace(artifactNameEntry.Text)
			if nl == "" || nw == "" {
				dialog.ShowError(errors.New("both runner fields are required"), u.window)
				return
			}
			if nb == "" {
				dialog.ShowError(errors.New("tool name cannot be empty"), u.window)
				return
			}
			saveConfigBtn.Disable()
			go func() {
				defer runOnUI(func() { saveConfigBtn.Enable() })
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
				if res := runGit(p.Path, "commit", "-m", "Update configuration via TDAManager"); res.Err != nil && !strings.Contains(res.Output, "nothing to commit") {
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

		configSection = sectionPanel("Build configuration",
			container.NewVBox(
				widget.NewForm(
					widget.NewFormItem("Tool name", artifactNameEntry),
					widget.NewFormItem("Linux runner", runnerLinuxEntry),
					widget.NewFormItem("Windows runner", runnerWindowsEntry),
				),
				container.NewBorder(nil, nil, nil, saveConfigBtn, widget.NewLabel("")),
			))
	}

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
		configSection,
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
