package launcher

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showTemplateSettings opens the dialog where the user changes the template
// repository URL used by every subsequent bootstrap.
func (u *ui) showTemplateSettings() {
	entry := widget.NewEntry()
	entry.SetText(u.store.TemplateURL())
	form := dialog.NewForm("Template repository", "Save", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Repository URL", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			_ = u.store.SetTemplateURL(strings.TrimSpace(entry.Text))
			u.state = u.store.Snapshot()
		}, u.window)
	form.Resize(fyne.NewSize(620, 140))
	form.Show()
}
