package launcher

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showPublishView builds the release form (version + changelog notes) and
// runs the publish() pipeline on confirm.
func (u *ui) showPublishView(p Project) {
	current := readLatestVersion(p.Path)

	// Pre-fill with a patch bump of the current version, or 1.0.0 for a fresh repo.
	maj, min, pat := "1", "0", "0"
	if current != "none" {
		parts := strings.SplitN(current, ".", 3)
		if len(parts) == 3 {
			maj, min, pat = parts[0], parts[1], parts[2]
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
