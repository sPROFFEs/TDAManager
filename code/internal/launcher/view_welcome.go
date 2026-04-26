package launcher

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showWelcomeView is the landing screen — empty state when no projects exist,
// otherwise a passive prompt to pick one from the sidebar.
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
