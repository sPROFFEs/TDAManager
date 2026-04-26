package launcher

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// askProviderBlocking prompts the user to pick a provider when auto-detection
// fails. Safe to call from a goroutine — internally schedules the dialog on
// the UI thread and waits on a channel.
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

// askYesNoBlocking is the goroutine-safe wrapper around dialog.ShowConfirm.
func (u *ui) askYesNoBlocking(title, body string) bool {
	ch := make(chan bool, 1)
	runOnUI(func() {
		dialog.ShowConfirm(title, body, func(ok bool) { ch <- ok }, u.window)
	})
	return <-ch
}

// runSignInWizard opens the per-provider PAT entry dialog, validates the
// token by attempting an authenticated ls-remote, and on success persists
// credentials via the per-repo credential helper. Returns true when sign-in
// completed successfully, false when the user cancelled or verification
// failed.
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
